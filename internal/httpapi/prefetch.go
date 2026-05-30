package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/neomat-prog/go-dicom-gateway/dicomfetch"
	"github.com/neomat-prog/go-dicom-gateway/source"
)

const (
	PrefetchStatusRunning = "running"

	PrefetchStatusCompleted = "completed"

	PrefetchStatusFailed = "failed"

	defaultSeriesBatchSize = 6
)

type PrefetchManager struct {
	lister  source.StudyLister
	fetcher *dicomfetch.Fetcher

	mut     sync.RWMutex
	nextID  int
	jobs    map[string]*PrefetchJob
	cancels map[string]context.CancelFunc

	onBatchStart func(batch int, series []source.SeriesInfo)
}

type PrefetchJob struct {
	JobID              string   `json:"jobId"`
	StudyInstanceUID   string   `json:"studyInstanceUID"`
	Status             string   `json:"status"`
	SeriesTotal        int      `json:"seriesTotal"`
	SeriesCompleted    int      `json:"seriesCompleted"`
	InstancesTotal     int      `json:"instancesTotal"`
	InstancesCompleted int      `json:"instancesCompleted"`
	BytesLoaded        int64    `json:"bytesLoaded"`
	CurrentBatch       int      `json:"currentBatch"`
	Errors             []string `json:"errors"`
}

type PrefetchRequest struct {
	SeriesInstanceUIDs []string `json:"seriesInstanceUIDs"`
	SeriesBatchSize    int      `json:"seriesBatchSize"`
}

type PrefetchStartResponse struct {
	JobID     string `json:"jobId"`
	Status    string `json:"status"`
	StatusURL string `json:"statusUrl"`
}

func NewPrefetchManager(lister source.StudyLister, fetcher *dicomfetch.Fetcher) *PrefetchManager {
	return &PrefetchManager{
		lister:  lister,
		fetcher: fetcher,
		jobs:    make(map[string]*PrefetchJob),
		cancels: make(map[string]context.CancelFunc),
	}
}

// Start creates a prefetch job for a study and starts it in the background.
func (m *PrefetchManager) Start(ctx context.Context, studyUID string, req PrefetchRequest) (PrefetchJob, error) {
	if m == nil || m.lister == nil || m.fetcher == nil {
		return PrefetchJob{}, errors.New("prefetch manager is not configured")
	}

	seriesList, err := m.lister.StudySeries(ctx, studyUID)
	if err != nil {
		return PrefetchJob{}, err
	}

	selected, err := selectPrefetchSeries(seriesList, req.SeriesInstanceUIDs)
	if err != nil {
		return PrefetchJob{}, err
	}

	batchSize := req.SeriesBatchSize
	if batchSize <= 0 {
		batchSize = defaultSeriesBatchSize
	}

	jobCtx, cancel := context.WithCancel(context.Background())

	job := PrefetchJob{
		JobID:            m.nextJobID(),
		StudyInstanceUID: studyUID,
		Status:           PrefetchStatusRunning,
		SeriesTotal:      len(selected),
		InstancesTotal:   countInstances(selected),
		CurrentBatch:     1,
	}

	m.mut.Lock()
	m.jobs[job.JobID] = &job
	m.cancels[job.JobID] = cancel
	m.mut.Unlock()

	go m.run(jobCtx, job.JobID, selected, batchSize)

	return clonePrefetchJob(job), nil
}

// Status returns a snapshot of one prefetch job.
func (m *PrefetchManager) Status(jobID string) (PrefetchJob, error) {
	m.mut.RLock()
	defer m.mut.RUnlock()

	job := m.jobs[jobID]
	if job == nil {
		return PrefetchJob{}, source.Wrap(source.ErrorKindNotFound, fmt.Errorf("prefetch job %q not found", jobID))
	}

	return clonePrefetchJob(*job), nil
}

// Delete cancels and removes one tracked prefetch job.
func (m *PrefetchManager) Delete(jobID string) error {
	m.mut.Lock()
	defer m.mut.Unlock()
	job := m.jobs[jobID]
	if job == nil {
		return source.Wrap(source.ErrorKindNotFound, fmt.Errorf("prefetch job %q not found", jobID))
	}
	if cancel := m.cancels[jobID]; cancel != nil {
		cancel()
	}
	delete(m.jobs, jobID)
	delete(m.cancels, jobID)
	return nil
}

// prefetchHandler starts a background prefetch job for a study.
func prefetchHandler(manager *PrefetchManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		studyUID := r.PathValue("studyUID")

		req, err := readPrefetchRequest(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		job, err := manager.Start(r.Context(), studyUID, req)
		if err != nil {
			writeSourceError(w, err)
			return
		}

		resp := PrefetchStartResponse{
			JobID:     job.JobID,
			Status:    job.Status,
			StatusURL: "/prefetch/" + job.JobID,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// prefetchStatusHandler returns the latest status for a prefetch job.
func prefetchStatusHandler(manager *PrefetchManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		job, err := manager.Status(r.PathValue("jobID"))
		if err != nil {
			writeSourceError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(job)
	}
}

// prefetchDeleteHandler cancels and removes a prefetch job.
func prefetchDeleteHandler(manager *PrefetchManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := manager.Delete(r.PathValue("jobID")); err != nil {
			writeSourceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (m *PrefetchManager) run(ctx context.Context, jobID string, seriesList []source.SeriesInfo, batchSize int) {
	if batchSize <= 0 {
		batchSize = len(seriesList)
	}

	for start := 0; start < len(seriesList); start += batchSize {
		end := min(start+batchSize, len(seriesList))

		batchNumber := start/batchSize + 1
		batchSeries := seriesList[start:end]

		m.setCurrentBatch(jobID, batchNumber)
		m.callBatchStart(batchNumber, batchSeries)

		var wg sync.WaitGroup
		for _, series := range batchSeries {
			series := series
			wg.Go(func() {
				m.prefetchSeries(ctx, jobID, series)
			})
		}
		wg.Wait()
	}

	m.finish(jobID)
}

func (m *PrefetchManager) callBatchStart(batch int, series []source.SeriesInfo) {
	if m.onBatchStart == nil {
		return
	}

	batchSeries := append([]source.SeriesInfo(nil), series...)
	m.onBatchStart(batch, batchSeries)
}

func (m *PrefetchManager) prefetchSeries(ctx context.Context, jobID string, series source.SeriesInfo) {
	refs := make([]source.InstanceRef, len(series.Instances))
	for i, inst := range series.Instances {
		refs[i] = inst.Ref
	}
	bytesLoaded, completed, err := m.fetcher.WarmSeries(ctx, refs)
	if err != nil {
		m.addError(jobID, fmt.Sprintf("series %s: %v", series.SeriesInstanceUID, err))
	}
	m.addSeriesProgress(jobID, completed, bytesLoaded)
}

func (m *PrefetchManager) nextJobID() string {
	m.mut.Lock()
	defer m.mut.Unlock()

	m.nextID++
	return fmt.Sprintf("prefetch-%d", m.nextID)
}

func (m *PrefetchManager) setCurrentBatch(jobID string, batch int) {
	m.mut.Lock()
	defer m.mut.Unlock()

	if job := m.jobs[jobID]; job != nil {
		job.CurrentBatch = batch
	}
}

func (m *PrefetchManager) addSeriesProgress(jobID string, instances int, bytesLoaded int64) {
	m.mut.Lock()
	defer m.mut.Unlock()

	if job := m.jobs[jobID]; job != nil {
		job.SeriesCompleted++
		job.InstancesCompleted += instances
		job.BytesLoaded += bytesLoaded
	}
}

func (m *PrefetchManager) addError(jobID string, msg string) {
	m.mut.Lock()
	defer m.mut.Unlock()

	if job := m.jobs[jobID]; job != nil {
		job.Errors = append(job.Errors, msg)
	}
}

func (m *PrefetchManager) finish(jobID string) {
	m.mut.Lock()
	defer m.mut.Unlock()

	job := m.jobs[jobID]
	if job == nil {
		return
	}

	if cancel := m.cancels[jobID]; cancel != nil {
		cancel()
		delete(m.cancels, jobID)
	}

	if len(job.Errors) > 0 {
		job.Status = PrefetchStatusFailed
		return
	}
	job.Status = PrefetchStatusCompleted
}

func readPrefetchRequest(body io.Reader) (PrefetchRequest, error) {
	var req PrefetchRequest
	if body == nil {
		return req, nil
	}

	decoder := json.NewDecoder(body)
	err := decoder.Decode(&req)
	if err != nil && !errors.Is(err, io.EOF) {
		return PrefetchRequest{}, err
	}

	return req, nil
}

func selectPrefetchSeries(seriesList []source.SeriesInfo, selectedUIDs []string) ([]source.SeriesInfo, error) {
	if len(selectedUIDs) == 0 {
		return seriesList, nil
	}

	seriesByUID := make(map[string]source.SeriesInfo, len(seriesList))
	for _, series := range seriesList {
		seriesByUID[series.SeriesInstanceUID] = series
	}

	selected := make([]source.SeriesInfo, 0, len(selectedUIDs))
	for _, uid := range selectedUIDs {
		series, ok := seriesByUID[uid]
		if !ok {
			return nil, source.Wrap(source.ErrorKindNotFound, fmt.Errorf("series %q not found", uid))
		}
		selected = append(selected, series)
	}

	return selected, nil
}

func countInstances(seriesList []source.SeriesInfo) int {
	total := 0
	for _, series := range seriesList {
		total += len(series.Instances)
	}
	return total
}

func clonePrefetchJob(job PrefetchJob) PrefetchJob {
	job.Errors = append([]string(nil), job.Errors...)
	return job
}
