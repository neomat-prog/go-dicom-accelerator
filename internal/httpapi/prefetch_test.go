package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/neomat-prog/go-dicom-gateway/dicomfetch"
	"github.com/neomat-prog/go-dicom-gateway/source"
)

func TestPrefetchManager_StartAllSeriesInBatches(t *testing.T) {
	src := newPrefetchTestSource("study-01", 16, 2)
	fetcher := dicomfetch.New(src, dicomfetch.Options{MaxConcurrency: 4})
	manager := NewPrefetchManager(src, fetcher)
	var batchSizes []int
	manager.onBatchStart = func(batch int, series []source.SeriesInfo) {
		batchSizes = append(batchSizes, len(series))
	}

	job, err := manager.Start(context.Background(), "study-01", PrefetchRequest{SeriesBatchSize: 6})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	got := waitForPrefetchStatus(t, manager, job.JobID, PrefetchStatusCompleted)

	if got.SeriesTotal != 16 {
		t.Fatalf("expected 16 total series, got %d", got.SeriesTotal)
	}
	if got.SeriesCompleted != 16 {
		t.Fatalf("expected 16 completed series, got %d", got.SeriesCompleted)
	}
	if got.InstancesTotal != 32 {
		t.Fatalf("expected 32 total instances, got %d", got.InstancesTotal)
	}
	if got.InstancesCompleted != 32 {
		t.Fatalf("expected 32 completed instances, got %d", got.InstancesCompleted)
	}
	if got.CurrentBatch != 3 {
		t.Fatalf("expected final batch 3 for 16 series with batch size 6, got %d", got.CurrentBatch)
	}
	assertInts(t, batchSizes, []int{6, 6, 4})
	if got.BytesLoaded == 0 {
		t.Fatalf("expected non-zero bytes loaded")
	}
	if src.callCount() != 32 {
		t.Fatalf("expected 32 source calls, got %d", src.callCount())
	}
}

func TestPrefetchManager_StartSelectedSeries(t *testing.T) {
	src := newPrefetchTestSource("study-01", 6, 2)
	fetcher := dicomfetch.New(src, dicomfetch.Options{MaxConcurrency: 4})
	manager := NewPrefetchManager(src, fetcher)

	job, err := manager.Start(context.Background(), "study-01", PrefetchRequest{
		SeriesInstanceUIDs: []string{"series-02", "series-04"},
		SeriesBatchSize:    6,
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	got := waitForPrefetchStatus(t, manager, job.JobID, PrefetchStatusCompleted)

	if got.SeriesTotal != 2 {
		t.Fatalf("expected 2 total series, got %d", got.SeriesTotal)
	}
	if got.InstancesTotal != 4 {
		t.Fatalf("expected 4 total instances, got %d", got.InstancesTotal)
	}

	calledSeries := src.calledSeriesUIDs()
	if len(calledSeries) != 2 || !calledSeries["series-02"] || !calledSeries["series-04"] {
		t.Fatalf("expected only selected series to be fetched, got %+v", calledSeries)
	}
}

func TestPrefetchRoutes_StartAndStatus(t *testing.T) {
	src := newPrefetchTestSource("study-01", 2, 2)
	fetcher := dicomfetch.New(src, dicomfetch.Options{MaxConcurrency: 2})
	manager := NewPrefetchManager(src, fetcher)
	mux := NewAcceleratedMux(src, "test", mockProber{}, src, fetcher, manager)

	body := bytes.NewBufferString(`{"seriesBatchSize":6}`)
	req := httptest.NewRequest(http.MethodPost, "/studies/study-01/prefetch", body)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected %d, got %d body=%q", http.StatusAccepted, w.Code, w.Body.String())
	}

	var started PrefetchStartResponse
	if err := json.NewDecoder(w.Body).Decode(&started); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if started.JobID == "" {
		t.Fatalf("expected job id")
	}
	if started.StatusURL != "/prefetch/"+started.JobID {
		t.Fatalf("unexpected status url %q", started.StatusURL)
	}

	waitForPrefetchStatus(t, manager, started.JobID, PrefetchStatusCompleted)

	statusReq := httptest.NewRequest(http.MethodGet, "/prefetch/"+started.JobID, nil)
	statusW := httptest.NewRecorder()

	mux.ServeHTTP(statusW, statusReq)

	if statusW.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d body=%q", http.StatusOK, statusW.Code, statusW.Body.String())
	}

	var status PrefetchJob
	if err := json.NewDecoder(statusW.Body).Decode(&status); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if status.Status != PrefetchStatusCompleted {
		t.Fatalf("expected completed status, got %q", status.Status)
	}
}

func TestPrefetchRoutes_InvalidStudyAndUnknownJob(t *testing.T) {
	src := newPrefetchTestSource("study-01", 1, 1)
	fetcher := dicomfetch.New(src, dicomfetch.Options{MaxConcurrency: 2})
	manager := NewPrefetchManager(src, fetcher)
	mux := NewAcceleratedMux(src, "test", mockProber{}, src, fetcher, manager)

	req := httptest.NewRequest(http.MethodPost, "/studies/missing-study/prefetch", strings.NewReader(`{}`))
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected %d for missing study, got %d", http.StatusNotFound, w.Code)
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/prefetch/missing-job", nil)
	statusW := httptest.NewRecorder()

	mux.ServeHTTP(statusW, statusReq)

	if statusW.Code != http.StatusNotFound {
		t.Fatalf("expected %d for unknown job, got %d", http.StatusNotFound, statusW.Code)
	}
}

type prefetchTestSource struct {
	mu     sync.Mutex
	series []source.SeriesInfo
	calls  []source.InstanceRef
}

func newPrefetchTestSource(studyUID string, seriesCount int, instancesPerSeries int) *prefetchTestSource {
	seriesList := make([]source.SeriesInfo, seriesCount)
	for seriesIndex := range seriesList {
		seriesUID := "series-" + twoDigitNumber(seriesIndex+1)
		instances := make([]source.InstanceInfo, instancesPerSeries)
		for instanceIndex := range instances {
			instances[instanceIndex] = source.InstanceInfo{
				Ref: source.InstanceRef{
					StudyInstanceUID:  studyUID,
					SeriesInstanceUID: seriesUID,
					SOPInstanceUID:    seriesUID + "-instance-" + twoDigitNumber(instanceIndex+1),
				},
				InstanceNumber:    instanceIndex + 1,
				HasInstanceNumber: true,
			}
		}

		seriesList[seriesIndex] = source.SeriesInfo{
			StudyInstanceUID:  studyUID,
			SeriesInstanceUID: seriesUID,
			Instances:         instances,
		}
	}

	return &prefetchTestSource{series: seriesList}
}

func (s *prefetchTestSource) StudyMetadata(ctx context.Context, studyUID string) (source.Metadata, error) {
	seriesList, err := s.StudySeries(ctx, studyUID)
	if err != nil {
		return source.Metadata{}, err
	}

	ref := seriesList[0].Instances[0].Ref
	return source.Metadata{
		StudyInstanceUID:  ref.StudyInstanceUID,
		SeriesInstanceUID: ref.SeriesInstanceUID,
		SOPInstanceUID:    ref.SOPInstanceUID,
	}, nil
}

func (s *prefetchTestSource) StudySeries(ctx context.Context, studyUID string) ([]source.SeriesInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var got []source.SeriesInfo
	for _, series := range s.series {
		if studyUID != "" && series.StudyInstanceUID != studyUID {
			continue
		}
		got = append(got, series)
	}

	if len(got) == 0 {
		return nil, source.Wrap(source.ErrorKindNotFound, io.ErrUnexpectedEOF)
	}
	return got, nil
}

func (s *prefetchTestSource) Instance(ctx context.Context, ref source.InstanceRef) (source.Response, error) {
	if err := ctx.Err(); err != nil {
		return source.Response{}, err
	}

	s.mu.Lock()
	s.calls = append(s.calls, ref)
	s.mu.Unlock()

	return source.Response{
		Body:          io.NopCloser(strings.NewReader(ref.SOPInstanceUID)),
		ContentType:   "application/dicom",
		ContentLength: int64(len(ref.SOPInstanceUID)),
		Filename:      ref.SOPInstanceUID + ".dcm",
	}, nil
}

func (s *prefetchTestSource) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *prefetchTestSource) calledSeriesUIDs() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	got := make(map[string]bool)
	for _, ref := range s.calls {
		got[ref.SeriesInstanceUID] = true
	}
	return got
}

func waitForPrefetchStatus(t *testing.T, manager *PrefetchManager, jobID string, want string) PrefetchJob {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, err := manager.Status(jobID)
		if err != nil {
			t.Fatalf("status returned error: %v", err)
		}
		if job.Status == want {
			return job
		}
		time.Sleep(time.Millisecond)
	}

	job, err := manager.Status(jobID)
	if err != nil {
		t.Fatalf("status returned error: %v", err)
	}
	t.Fatalf("expected status %q, got %q", want, job.Status)
	return PrefetchJob{}
}

func twoDigitNumber(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func assertInts(t *testing.T, got []int, want []int) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected %d ints, got %d: %+v", len(want), len(got), got)
	}

	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("int %d: expected %d, got %d", i, want[i], got[i])
		}
	}
}

var _ source.Source = (*prefetchTestSource)(nil)
var _ source.StudyLister = (*prefetchTestSource)(nil)
