package gcs

import (
	"context"
	"testing"
	"time"

	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/neomat-prog/go-dicom-gateway/source"
)

const testBucket = "my-bucket"

func fakeServer(t *testing.T) *fakestorage.Server {
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{Scheme: "http"})
	if err != nil {
		t.Fatalf("create fake gcs server: %v", err)
	}
	t.Cleanup(server.Stop)
	t.Setenv("STORAGE_EMULATOR_HOST", server.URL())
	return server
}

// obj builds a .dcm
func obj(study, series, sop string) fakestorage.Object {
	return fakestorage.Object{
		ObjectAttrs: fakestorage.ObjectAttrs{
			BucketName: testBucket,
			Name:       "dicom/" + study + "/" + series + "/" + sop + ".dcm",
		},
		Content: []byte("fake"),
	}
}

func TestNew(t *testing.T) {
	fakeServer(t)
	src, err := New(context.Background(), testBucket, "dicom/")
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if src.prefix != "dicom/" {
		t.Errorf("prefix = %q, want %q", src.prefix, "dicom/")
	}
}

func TestLoadIndexTTL(t *testing.T) {
	ctx := context.Background()
	server := fakeServer(t)
	server.CreateObject(obj("1.1", "2.1", "3.1"))
	server.CreateObject(obj("1.1", "2.1", "3.2"))

	src, err := New(ctx, testBucket, "dicom/")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	fake := time.Unix(0, 0)
	src.now = func() time.Time { return fake }
	src.indexTTL = 30 * time.Second

	idx, err := src.loadIndex(ctx)
	if err != nil {
		t.Fatalf("loadIndex: %v", err)
	}
	if len(idx.all) != 2 {
		t.Fatalf("initial: got %d, want 2", len(idx.all))
	}

	server.CreateObject(obj("1.1", "2.1", "3.3"))
	fake = fake.Add(10 * time.Second)
	idx, _ = src.loadIndex(ctx)
	if len(idx.all) != 2 {
		t.Fatalf("within TTL: got %d, want cached 2", len(idx.all))
	}

	fake = fake.Add(30 * time.Second)
	idx, _ = src.loadIndex(ctx)
	if len(idx.all) != 3 {
		t.Fatalf("past TTL: got %d, want 3", len(idx.all))
	}
}

func TestSeriesInstances(t *testing.T) {
	ctx := context.Background()
	server := fakeServer(t)
	server.CreateObject(obj("study1", "seriesA", "sop1"))
	server.CreateObject(obj("study1", "seriesA", "sop2"))
	server.CreateObject(obj("study1", "seriesB", "sop3"))
	server.CreateObject(obj("study2", "seriesC", "sop4"))
	// noise: non-.dcm + malformed → must be ignored
	server.CreateObject(fakestorage.Object{
		ObjectAttrs: fakestorage.ObjectAttrs{BucketName: testBucket, Name: "dicom/readme.txt"},
		Content:     []byte("x"),
	})

	src, err := New(ctx, testBucket, "dicom/")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got, err := src.SeriesInstances(ctx, "study1", "seriesA")
	if err != nil {
		t.Fatalf("SeriesInstances: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("study1/seriesA: got %d, want 2", len(got))
	}

	_, err = src.SeriesInstances(ctx, "nope", "nope")
	if !source.IsKind(err, source.ErrorKindNotFound) {
		t.Errorf("missing series: got %v, want NotFound", err)
	}
}

func TestStudySeries(t *testing.T) {
	ctx := context.Background()
	server := fakeServer(t)
	server.CreateObject(obj("study1", "seriesA", "sop1"))
	server.CreateObject(obj("study1", "seriesB", "sop2"))

	src, err := New(ctx, testBucket, "dicom/")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got, err := src.StudySeries(ctx, "study1")
	if err != nil {
		t.Fatalf("StudySeries: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("study1: got %d series, want 2", len(got))
	}

	_, err = src.StudySeries(ctx, "missing")
	if !source.IsKind(err, source.ErrorKindNotFound) {
		t.Errorf("missing study: got %v, want NotFound", err)
	}
}
