package gcs

import (
	"context"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/fsouza/fake-gcs-server/fakestorage"
)

func TestNew(t *testing.T) {
	ctx := context.Background()

	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{Scheme: "http"})
	if err != nil {
		t.Fatalf("create fake gcs server: %v", err)
	}
	defer server.Stop()
	t.Setenv("STORAGE_EMULATOR_HOST", server.URL())

	client, err := storage.NewClient(ctx)
	if err != nil {
		t.Fatalf("create emulator gcs server %v", err)
	}
	defer client.Close()

	if err := client.Bucket("my-bucket").Create(ctx, "test-project", nil); err != nil {
		t.Fatalf("create bucket: %v", err)
	}

	src, err := New(ctx, "my-bucket", "dicom/")
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	if src == nil {
		t.Fatal("expected source")
	}

	if src.prefix != "dicom/" {
		t.Errorf("expected prefix %q, got %q", "dicom/", src.prefix)
	}
}
