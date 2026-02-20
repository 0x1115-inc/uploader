package main

import (
	"log"
	"net/http"
	"os"
	"time"

	httpapi "uploader/internal/http"
	"uploader/internal/storage"
	"uploader/internal/db"
)

func main() {
	port := getenv("PORT", "8080")

	srv := httpapi.NewServer(
		httpapi.Config{MaxUploadMB: 50},
		db.NewStub(),
		storage.NewStub(),
	)

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("uploader listening on :%s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

