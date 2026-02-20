package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"uploader/internal/db"
	httpapi "uploader/internal/http"
	"uploader/internal/logx"
	"uploader/internal/storage"
)

func main() {
	cfg := loadConfig()
	logx.SetLevelFromString(cfg.LogLevel)
	logx.Infof("log level set to %s", cfg.LogLevel)

	sqliteStore, err := db.NewSQLite(cfg.DBPath)
	if err != nil {
		logx.Errorf("db init failed: %v", err)
		os.Exit(1)
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
		logx.Errorf("storage init failed: %v", err)
		os.Exit(1)
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
		logx.Infof("uploader listening on :%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logx.Errorf("server failed: %v", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logx.Warnf("shutdown error: %v", err)
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
		logx.Warnf("storage init attempt %d/30 failed: %v", i, err)
		time.Sleep(2 * time.Second)
	}
	if lastErr == nil {
		lastErr = errors.New("unknown storage init error")
	}
	return nil, lastErr
}
