package dicomfetch

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/neomat-prog/go-dicom-gateway/source"
)

func TestSelectWindow(t *testing.T) {
	refs := testRefs(10)

	got, err := SelectWindow(refs, 4, 2, 3)
	if err != nil {
		t.Fatalf("SelectWindow returned error: %v", err)
	}

	assertSOPUIDs(t, got, []string{
		"instance-02",
		"instance-03",
		"instance-04",
		"instance-05",
		"instance-06",
		"instance-07",
	})
}

func TestSelectWindow_ClampsAtStart(t *testing.T) {
	refs := testRefs(10)

	got, err := SelectWindow(refs, 0, 4, 2)
	if err != nil {
		t.Fatalf("SelectWindow returned error: %v", err)
	}

	assertSOPUIDs(t, got, []string{
		"instance-00",
		"instance-01",
		"instance-02",
	})
}

func TestSelectWindow_ClampsAtEnd(t *testing.T) {
	refs := testRefs(10)

	got, err := SelectWindow(refs, 9, 2, 4)
	if err != nil {
		t.Fatalf("SelectWindow returned error: %v", err)
	}

	assertSOPUIDs(t, got, []string{
		"instance-07",
		"instance-08",
		"instance-09",
	})
}

func TestFetchInstance_ReadsBodyClosesBackendAndCaches(t *testing.T) {
	src := &trackingSource{}
	fetcher := New(src, Options{MaxConcurrency: 2})
	ref := testRefs(1)[0]

	first, err := fetcher.FetchInstance(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchInstance returned error: %v", err)
	}

	if string(first.Data) != "instance-00" {
		t.Fatalf("expected fetched data from source, got %q", string(first.Data))
	}
	if src.closeCount() != 1 {
		t.Fatalf("expected backend body to be closed once, got %d", src.closeCount())
	}

	first.Data[0] = 'X'

	second, err := fetcher.FetchInstance(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchInstance returned error from cache: %v", err)
	}

	if string(second.Data) != "instance-00" {
		t.Fatalf("expected cached data to be protected from mutation, got %q", string(second.Data))
	}
	if src.callCount() != 1 {
		t.Fatalf("expected second fetch to use cache, got %d source calls", src.callCount())
	}
}

func TestFetchWindow_UsesBoundedConcurrency(t *testing.T) {
	src := &trackingSource{delay: 10 * time.Millisecond}
	fetcher := New(src, Options{
		MaxConcurrency: 2,
		WindowBehind:   1,
		WindowAhead:    3,
		RequestTimeout: time.Second,
	})

	got, err := fetcher.FetchWindow(context.Background(), testRefs(8), 2)
	if err != nil {
		t.Fatalf("FetchWindow returned error: %v", err)
	}

	if len(got) != 5 {
		t.Fatalf("expected 5 fetched instances, got %d", len(got))
	}
	if src.maxActiveRequests() > 2 {
		t.Fatalf("expected at most 2 active requests, saw %d", src.maxActiveRequests())
	}

	assertFetchedSOPUIDs(t, got, []string{
		"instance-01",
		"instance-02",
		"instance-03",
		"instance-04",
		"instance-05",
	})
}

func TestFetchWindow_ConcurrentIsFasterThanSequentialWhenSourceHasLatency(t *testing.T) {
	refs := testRefs(12)
	delay := 25 * time.Millisecond

	sequentialDuration := timedFetchWindow(t, &trackingSource{delay: delay}, Options{
		MaxConcurrency: 1,
		WindowBehind:   5,
		WindowAhead:    6,
		RequestTimeout: 2 * time.Second,
	}, refs, 5)

	concurrentDuration := timedFetchWindow(t, &trackingSource{delay: delay}, Options{
		MaxConcurrency: 4,
		WindowBehind:   5,
		WindowAhead:    6,
		RequestTimeout: 2 * time.Second,
	}, refs, 5)

	t.Logf("sequential duration=%s concurrent duration=%s", sequentialDuration, concurrentDuration)

	if concurrentDuration >= sequentialDuration {
		t.Fatalf("expected concurrent fetch to be faster than sequential fetch")
	}

	if concurrentDuration > sequentialDuration*3/4 {
		t.Fatalf("expected concurrent fetch to be meaningfully faster: sequential=%s concurrent=%s", sequentialDuration, concurrentDuration)
	}
}

type trackingSource struct {
	mu        sync.Mutex
	active    int
	maxActive int
	calls     []string
	closes    int
	delay     time.Duration
}

func (s *trackingSource) StudyMetadata(ctx context.Context, studyUID string) (source.Metadata, error) {
	return source.Metadata{StudyInstanceUID: studyUID}, nil
}

func (s *trackingSource) Instance(ctx context.Context, ref source.InstanceRef) (source.Response, error) {
	s.mu.Lock()
	s.active++
	if s.active > s.maxActive {
		s.maxActive = s.active
	}
	s.calls = append(s.calls, ref.SOPInstanceUID)
	s.mu.Unlock()

	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			s.finishRequest()
			return source.Response{}, ctx.Err()
		}
	}

	body := &trackingBody{
		Reader: strings.NewReader(ref.SOPInstanceUID),
		onClose: func() {
			s.mu.Lock()
			s.closes++
			s.mu.Unlock()
			s.finishRequest()
		},
	}

	return source.Response{
		Body:          body,
		ContentType:   "application/dicom",
		ContentLength: int64(len(ref.SOPInstanceUID)),
		Filename:      ref.SOPInstanceUID + ".dcm",
	}, nil
}

func (s *trackingSource) finishRequest() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active--
}

func (s *trackingSource) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *trackingSource) closeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closes
}

func (s *trackingSource) maxActiveRequests() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxActive
}

type trackingBody struct {
	*strings.Reader
	onClose func()
}

func (b *trackingBody) Close() error {
	if b.onClose != nil {
		b.onClose()
		b.onClose = nil
	}
	return nil
}

func testRefs(count int) []source.InstanceRef {
	refs := make([]source.InstanceRef, count)
	for i := range refs {
		refs[i] = source.InstanceRef{
			StudyInstanceUID:  "study-01",
			SeriesInstanceUID: "series-01",
			SOPInstanceUID:    fmt.Sprintf("instance-%02d", i),
		}
	}
	return refs
}

func timedFetchWindow(t *testing.T, src source.Source, options Options, refs []source.InstanceRef, center int) time.Duration {
	t.Helper()

	fetcher := New(src, options)

	start := time.Now()
	got, err := fetcher.FetchWindow(context.Background(), refs, center)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("FetchWindow returned error: %v", err)
	}

	if len(got) != len(refs) {
		t.Fatalf("expected %d fetched instances, got %d", len(refs), len(got))
	}

	return elapsed
}

func assertSOPUIDs(t *testing.T, refs []source.InstanceRef, want []string) {
	t.Helper()

	if len(refs) != len(want) {
		t.Fatalf("expected %d refs, got %d", len(want), len(refs))
	}

	for i := range refs {
		if refs[i].SOPInstanceUID != want[i] {
			t.Fatalf("ref %d: expected %q, got %q", i, want[i], refs[i].SOPInstanceUID)
		}
	}
}

func assertFetchedSOPUIDs(t *testing.T, instances []FetchedInstance, want []string) {
	t.Helper()

	if len(instances) != len(want) {
		t.Fatalf("expected %d instances, got %d", len(want), len(instances))
	}

	for i := range instances {
		if instances[i].Ref.SOPInstanceUID != want[i] {
			t.Fatalf("instance %d: expected %q, got %q", i, want[i], instances[i].Ref.SOPInstanceUID)
		}
		data, err := ReadAllAndClose(instances[i].Response())
		if err != nil {
			t.Fatalf("read response %d: %v", i, err)
		}
		if string(data) != want[i] {
			t.Fatalf("response %d: expected body %q, got %q", i, want[i], string(data))
		}
	}
}

var _ source.Source = (*trackingSource)(nil)
var _ io.ReadCloser = (*trackingBody)(nil)
