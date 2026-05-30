package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/suyashkumar/dicom"
	dicomtag "github.com/suyashkumar/dicom/pkg/tag"
)

type LocalDirectorySource struct {
	Root string
}

func NewLocalDirectory(root string) *LocalDirectorySource {
	return &LocalDirectorySource{Root: root}
}

// Probe verifies that the configured root exists and is a directory.
func (s *LocalDirectorySource) Probe(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(s.Root)
	if err != nil {
		return Wrap(ErrorKindUpstream, err)
	}
	if !info.IsDir() {
		return Wrap(ErrorKindUpstream, fmt.Errorf("%s is not a directory", s.Root))
	}
	return nil
}

// StudyMetadata returns identifiers from the first matching local instance.
func (s *LocalDirectorySource) StudyMetadata(ctx context.Context, studyUID string) (Metadata, error) {
	instances, err := s.SeriesInstances(ctx, studyUID, "")
	if err != nil {
		return Metadata{}, err
	}

	ref := instances[0].Ref

	return Metadata{
		StudyInstanceUID:  ref.StudyInstanceUID,
		SeriesInstanceUID: ref.SeriesInstanceUID,
		SOPInstanceUID:    ref.SOPInstanceUID,
	}, nil
}

// SeriesInstances lists instances matching studyUID and seriesUID.
func (s *LocalDirectorySource) SeriesInstances(ctx context.Context, studyUID string, seriesUID string) ([]InstanceInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	paths, err := dicomFilePaths(s.Root)
	if err != nil {
		return nil, err
	}

	var instances []InstanceInfo

	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		info, err := readLocalInstanceInfo(path)
		if err != nil {
			return nil, Wrap(ErrorKindUpstream, err)
		}

		ref := info.Ref

		if studyUID != "" && ref.StudyInstanceUID != studyUID {
			continue
		}

		if seriesUID != "" && ref.SeriesInstanceUID != seriesUID {
			continue
		}

		instances = append(instances, info)
	}

	if len(instances) == 0 {
		return nil, Wrap(ErrorKindNotFound, fmt.Errorf("series not found"))
	}

	SortInstanceInfos(instances)

	return instances, nil
}

// Instance opens the local DICOM file matching ref.
func (s *LocalDirectorySource) Instance(ctx context.Context, ref InstanceRef) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}

	paths, err := dicomFilePaths(s.Root)
	if err != nil {
		return Response{}, err
	}

	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return Response{}, err
		}

		info, err := readLocalInstanceInfo(path)
		if err != nil {
			return Response{}, Wrap(ErrorKindUpstream, err)
		}

		if !matchesRef(info.Ref, ref) {
			continue
		}

		file, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				return Response{}, Wrap(ErrorKindNotFound, err)
			}
			return Response{}, Wrap(ErrorKindUpstream, err)
		}

		stat, err := file.Stat()
		if err != nil {
			file.Close()
			return Response{}, Wrap(ErrorKindUpstream, err)
		}

		return Response{
			Body:          file,
			ContentType:   "application/dicom",
			ContentLength: stat.Size(),
			Filename:      filepath.Base(path),
		}, nil
	}

	return Response{}, Wrap(ErrorKindNotFound, fmt.Errorf("instance not found"))
}

// StudySeries groups local instances by study and series identifiers.
func (s *LocalDirectorySource) StudySeries(ctx context.Context, studyUID string) ([]SeriesInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	paths, err := dicomFilePaths(s.Root)
	if err != nil {
		return nil, err
	}

	seriesByUID := make(map[string]*SeriesInfo)

	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		info, err := readLocalInstanceInfo(path)
		if err != nil {
			return nil, Wrap(ErrorKindUpstream, err)
		}

		ref := info.Ref

		if studyUID != "" && ref.StudyInstanceUID != studyUID {
			continue
		}

		key := ref.StudyInstanceUID + "\x00" + ref.SeriesInstanceUID

		series, ok := seriesByUID[key]
		if !ok {
			series = &SeriesInfo{
				StudyInstanceUID:  ref.StudyInstanceUID,
				SeriesInstanceUID: ref.SeriesInstanceUID,
			}
			seriesByUID[key] = series
		}

		series.Instances = append(series.Instances, info)
	}

	if len(seriesByUID) == 0 {
		return nil, Wrap(ErrorKindNotFound, fmt.Errorf("study not found"))
	}

	seriesList := make([]SeriesInfo, 0, len(seriesByUID))

	for _, series := range seriesByUID {
		SortInstanceInfos(series.Instances)
		seriesList = append(seriesList, *series)
	}

	SortSeriesList(seriesList)

	return seriesList, nil
}

// SortSeriesList orders series by SeriesInstanceUID.
func SortSeriesList(seriesList []SeriesInfo) {
	sort.SliceStable(seriesList, func(i, j int) bool {
		return seriesList[i].SeriesInstanceUID < seriesList[j].SeriesInstanceUID
	})
}

// SortInstanceInfos orders instances by InstanceNumber when available, then by
// SOPInstanceUID.
func SortInstanceInfos(instances []InstanceInfo) {
	sort.SliceStable(instances, func(i, j int) bool {
		left := instances[i]
		right := instances[j]

		if left.HasInstanceNumber && right.HasInstanceNumber {
			if left.InstanceNumber != right.InstanceNumber {
				return left.InstanceNumber < right.InstanceNumber
			}
		}

		if left.HasInstanceNumber != right.HasInstanceNumber {
			return left.HasInstanceNumber
		}

		return left.Ref.SOPInstanceUID < right.Ref.SOPInstanceUID
	})
}

func dicomFilePaths(root string) ([]string, error) {
	var paths []string

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			return nil
		}

		if strings.ToLower(filepath.Ext(entry.Name())) != ".dcm" {
			return nil
		}

		paths = append(paths, path)
		return nil
	})

	if err != nil {
		if os.IsNotExist(err) {
			return nil, Wrap(ErrorKindNotFound, err)
		}
		return nil, Wrap(ErrorKindUpstream, err)
	}

	if len(paths) == 0 {
		return nil, Wrap(ErrorKindNotFound, fmt.Errorf("no dicom files foind in %s", root))
	}

	sort.Strings(paths)

	return paths, nil
}

func readLocalInstanceInfo(path string) (InstanceInfo, error) {
	ds, err := dicom.ParseFile(path, nil, dicom.SkipPixelData())
	if err != nil {
		return InstanceInfo{}, fmt.Errorf("parse dicom %s: %w", path, err)
	}

	studyUID, err := readStringTag(ds, dicomtag.StudyInstanceUID)
	if err != nil {
		return InstanceInfo{}, err
	}

	seriesUID, err := readStringTag(ds, dicomtag.SeriesInstanceUID)
	if err != nil {
		return InstanceInfo{}, err
	}

	instanceUID, err := readStringTag(ds, dicomtag.SOPInstanceUID)
	if err != nil {
		return InstanceInfo{}, err
	}

	instanceNumber, hasInstanceNumber, err := readOptionalIntTag(ds, dicomtag.InstanceNumber)
	if err != nil {
		return InstanceInfo{}, err
	}

	return InstanceInfo{
		Ref: InstanceRef{
			StudyInstanceUID:  studyUID,
			SeriesInstanceUID: seriesUID,
			SOPInstanceUID:    instanceUID,
		},
		InstanceNumber:    instanceNumber,
		HasInstanceNumber: hasInstanceNumber,
	}, nil
}

func readOptionalIntTag(ds dicom.Dataset, t dicomtag.Tag) (int, bool, error) {
	elem, err := ds.FindElementByTag(t)
	if err != nil {
		return 0, false, nil
	}

	values := dicom.MustGetStrings(elem.Value)
	if len(values) == 0 {
		return 0, false, nil
	}

	raw := strings.TrimSpace(values[0])
	if raw == "" {
		return 0, false, nil
	}

	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, true, fmt.Errorf("tag %v has invalid integer value %q: %w", t, raw, err)
	}

	return n, true, nil
}

func matchesRef(got InstanceRef, want InstanceRef) bool {
	if want.StudyInstanceUID != "" && got.StudyInstanceUID != want.StudyInstanceUID {
		return false
	}

	if want.SeriesInstanceUID != "" && got.SeriesInstanceUID != want.SeriesInstanceUID {
		return false
	}

	if want.SOPInstanceUID != "" && got.SOPInstanceUID != want.SOPInstanceUID {
		return false
	}

	return true
}
