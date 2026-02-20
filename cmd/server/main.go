package main

import (
	"context"
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

	srv := httpapi.NewServer(
		httpapi.Config{MaxUploadMB: cfg.MaxUploadMB},
		sqliteStore,
		storage.NewStub(),
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
