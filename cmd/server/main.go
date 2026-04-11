package main

import (
	"log"
	"net/http"

	"github.com/neomat-prog/dicom-retrieval-accelerator/internal/httpapi"
)

func main() {
	server := &http.Server{
		Addr:    ":8080",
		Handler: httpapi.NewMux(),
	}

	log.Println("Starting server on :8080")
	log.Fatal(server.ListenAndServe())
}
