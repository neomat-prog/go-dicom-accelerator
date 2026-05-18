package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/suyashkumar/dicom"
	dicomtag "github.com/suyashkumar/dicom/pkg/tag"
)

func main() {
	bucket := flag.String("bucket", "", "GCS bucket name (required)")
	source := flag.String("source", "./sample-dicom", "local directory of .dcm files")
	prefix := flag.String("prefix", "", "optional GCS path prefix, e.g. studies/")
	flag.Parse()

	if *bucket == "" {
		log.Fatal("--bucket is required")
	}

	ctx := context.Background()

	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("create gcs client: %v", err)
	}
	defer client.Close()

	bkt := client.Bucket(*bucket)

	var uploaded, skipped int

	err = filepath.WalkDir(*source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".dcm" {
			return nil
		}

		studyUID, seriesUID, sopUID, err := readUIDs(path)
		if err != nil {
			log.Printf("skip %s: %v", path, err)
			skipped++
			return nil
		}

		objectName := *prefix + studyUID + "/" + seriesUID + "/" + sopUID + ".dcm"

		if err := uploadFile(ctx, bkt, objectName, path); err != nil {
			return fmt.Errorf("upload %s: %w", path, err)
		}

		log.Printf("uploaded gs://%s/%s", *bucket, objectName)
		uploaded++
		return nil
	})

	if err != nil {
		log.Fatalf("walk error: %v", err)
	}

	log.Printf("done: %d uploaded, %d skipped", uploaded, skipped)
}

func readUIDs(path string) (studyUID, seriesUID, sopUID string, err error) {
	ds, err := dicom.ParseFile(path, nil, dicom.SkipPixelData())
	if err != nil {
		return "", "", "", fmt.Errorf("parse dicom: %w", err)
	}

	studyUID, err = stringTag(ds, dicomtag.StudyInstanceUID)
	if err != nil {
		return "", "", "", err
	}
	seriesUID, err = stringTag(ds, dicomtag.SeriesInstanceUID)
	if err != nil {
		return "", "", "", err
	}
	sopUID, err = stringTag(ds, dicomtag.SOPInstanceUID)
	if err != nil {
		return "", "", "", err
	}
	return studyUID, seriesUID, sopUID, nil
}

func stringTag(ds dicom.Dataset, t dicomtag.Tag) (string, error) {
	elem, err := ds.FindElementByTag(t)
	if err != nil {
		return "", fmt.Errorf("missing tag %v: %w", t, err)
	}
	values := dicom.MustGetStrings(elem.Value)
	if len(values) == 0 {
		return "", fmt.Errorf("tag %v has no value", t)
	}
	return strings.TrimSpace(values[0]), nil
}

func uploadFile(ctx context.Context, bkt *storage.BucketHandle, objectName, localPath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bkt.Object(objectName).NewWriter(ctx)
	w.ContentType = "application/dicom"

	if _, err := io.Copy(w, f); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}
