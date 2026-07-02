package gcs

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/fsouza/fake-gcs-server/fakestorage"
)

func fakeServer(t *testing.T) *fakestorage.Server {
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{Scheme: "http"})
	if err != nil {
		log.Fatalf("create server: %v", err)
	}
	t.Cleanup(server.Stop)
	t.Setenv("STORAGE_EMULATOR_HOST", server.URL())
	return server
}

func TestNew(t *testing.T) {
	fakeServer(t)
	src, err := New(context.Background(), "my-bucket", "dicom/")
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if src.prefix != "dicom/" {
		t.Errorf("prefix = %q, want %q", src.prefix, "dicom/")
	}
}

func TestLoadIndexTTL(t *testing.T) {
	obj := func(study, series, sop string) fakestorage.Object {
		return fakestorage.Object{
			ObjectAttrs: fakestorage.ObjectAttrs{
				BucketName: "my-bucket",
				Name:       "dicom/" + study + "/" + series + "/" + sop + ".dcm",
			},
			Content: []byte("fake"),
		}
	}

	ctx := context.Background()
	server := fakeServer(t)
	server.CreateObject(obj("1.1", "2.1", "3.1"))
	server.CreateObject(obj("1.1", "2.1", "3.2"))

	src, err := New(ctx, "my-bucket", "dicom/")
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
		t.Fatalf("got %d, want 2", len(idx.all))
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
