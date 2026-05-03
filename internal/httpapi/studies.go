package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/neomat-prog/go-dicom-gateway/dicomfetch"
	"github.com/neomat-prog/go-dicom-gateway/source"
)

func acceleratedInstanceHandler(lister source.StudyLister, fetcher *dicomfetch.Fetcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		studyUID := r.PathValue("studyUID")
		seriesUID := r.PathValue("seriesUID")
		instanceUID := r.PathValue("instanceUID")

		seriesList, err := lister.StudySeries(r.Context(), studyUID)
		if err != nil {
			writeSourceError(w, err)
			return
		}

		instances, err := findSeries(seriesList, seriesUID)
		if err != nil {
			writeSourceError(w, err)
			return
		}

		refs := make([]source.InstanceRef, len(instances))
		center := -1

		for i, info := range instances {
			refs[i] = info.Ref
			if info.Ref.SOPInstanceUID == instanceUID {
				center = i
			}
		}

		if center == -1 {
			writeSourceError(w, source.Wrap(source.ErrorKindNotFound, fmt.Errorf("instance %q not found", instanceUID)))
			return
		}

		window, err := fetcher.FetchWindow(r.Context(), refs, center)
		if err != nil {
			writeSourceError(w, err)
			return
		}

		instance, err := findFetchedInstance(window, instanceUID)
		if err != nil {
			writeSourceError(w, err)
			return
		}

		resp := instance.Response()
		defer resp.Body.Close()

		w.Header().Set("Content-Type", resp.ContentType)
		w.Header().Set("Content-Disposition", "inline; filename="+strconv.Quote(resp.Filename))
		w.Header().Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
		w.WriteHeader(http.StatusOK)

		_, _ = io.Copy(w, resp.Body)
	}
}

func studiesHandler(lister source.StudyLister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		seriesList, err := lister.StudySeries(r.Context(), "")
		if err != nil {
			writeSourceError(w, err)
			return
		}

		type studyResponse struct {
			StudyInstanceUID string `json:"studyInstanceUID"`
			SeriesCount      int    `json:"seriesCount"`
			InstanceCount    int    `json:"instanceCount"`
		}

		studiesByUID := make(map[string]*studyResponse)
		for _, series := range seriesList {
			study := studiesByUID[series.StudyInstanceUID]
			if study == nil {
				study = &studyResponse{StudyInstanceUID: series.StudyInstanceUID}
				studiesByUID[series.StudyInstanceUID] = study
			}

			study.SeriesCount++
			study.InstanceCount += len(series.Instances)
		}

		resp := make([]studyResponse, 0, len(studiesByUID))
		for _, study := range studiesByUID {
			resp = append(resp, *study)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func studySeriesHandler(lister source.StudyLister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		studyUID := r.PathValue("studyUID")

		seriesList, err := lister.StudySeries(r.Context(), studyUID)
		if err != nil {
			writeSourceError(w, err)
			return
		}

		type seriesResponse struct {
			StudyInstanceUID  string `json:"studyInstanceUID"`
			SeriesInstanceUID string `json:"seriesInstanceUID"`
			InstanceCount     int    `json:"instanceCount"`
		}

		resp := make([]seriesResponse, len(seriesList))
		for i, series := range seriesList {
			resp[i] = seriesResponse{
				StudyInstanceUID:  series.StudyInstanceUID,
				SeriesInstanceUID: series.SeriesInstanceUID,
				InstanceCount:     len(series.Instances),
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func seriesInstancesHandler(lister source.StudyLister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		studyUID := r.PathValue("studyUID")
		seriesUID := r.PathValue("seriesUID")

		seriesList, err := lister.StudySeries(r.Context(), studyUID)
		if err != nil {
			writeSourceError(w, err)
			return
		}

		instances, err := findSeries(seriesList, seriesUID)
		if err != nil {
			writeSourceError(w, err)
			return
		}

		type instanceResponse struct {
			StudyInstanceUID  string `json:"studyInstanceUID"`
			SeriesInstanceUID string `json:"seriesInstanceUID"`
			SOPInstanceUID    string `json:"sopInstanceUID"`
			InstanceNumber    int    `json:"instanceNumber,omitempty"`
		}

		resp := make([]instanceResponse, len(instances))
		for i, instance := range instances {
			resp[i] = instanceResponse{
				StudyInstanceUID:  instance.Ref.StudyInstanceUID,
				SeriesInstanceUID: instance.Ref.SeriesInstanceUID,
				SOPInstanceUID:    instance.Ref.SOPInstanceUID,
			}
			if instance.HasInstanceNumber {
				resp[i].InstanceNumber = instance.InstanceNumber
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func findSeries(seriesList []source.SeriesInfo, seriesUID string) ([]source.InstanceInfo, error) {
	for _, series := range seriesList {
		if series.SeriesInstanceUID == seriesUID {
			return series.Instances, nil
		}
	}

	return nil, source.Wrap(source.ErrorKindNotFound, fmt.Errorf("series %q not found", seriesUID))
}

func findFetchedInstance(instances []dicomfetch.FetchedInstance, instanceUID string) (dicomfetch.FetchedInstance, error) {
	for _, instance := range instances {
		if instance.Ref.SOPInstanceUID == instanceUID {
			return instance, nil
		}
	}

	return dicomfetch.FetchedInstance{}, source.Wrap(source.ErrorKindNotFound, fmt.Errorf("instance %q not found after fetch", instanceUID))
}
