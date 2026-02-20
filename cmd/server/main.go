package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"uploader/internal/db"
	httpapi "uploader/internal/http"
	"uploader/internal/storage"
)

func main() {
	cfg := loadConfig()

	sqliteStore, err := db.NewSQLite(cfg.DBPath)
	if err != nil {
		log.Fatalf("db init failed: %v", err)
	}
	defer func() { _ = sqliteStore.Close() }()

	storageClient, err := newS3WithRetry(storage.S3Config{
		Endpoint:  cfg.S3Endpoint,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		Bucket:    cfg.S3Bucket,
		Region:    cfg.S3Region,
		UseSSL:    cfg.S3UseSSL,
	})
	if err != nil {
		log.Fatalf("storage init failed: %v", err)
	}

	srv := httpapi.NewServer(
		httpapi.Config{MaxUploadMB: cfg.MaxUploadMB, S3Bucket: cfg.S3Bucket},
		sqliteStore,
		storageClient,
	)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("uploader listening on :%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

func newS3WithRetry(cfg storage.S3Config) (*storage.S3Provider, error) {
	var lastErr error
	for i := 1; i <= 30; i++ {
		client, err := storage.NewS3(cfg)
		if err == nil {
			return client, nil
		}
		lastErr = err
		log.Printf("storage init attempt %d/30 failed: %v", i, err)
		time.Sleep(2 * time.Second)
	}
	if lastErr == nil {
		lastErr = errors.New("unknown storage init error")
	}
	return nil, lastErr
}
