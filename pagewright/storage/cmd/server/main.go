package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/PageWrightCloud/pagewright/storage/internal/api"
	"github.com/PageWrightCloud/pagewright/storage/internal/config"
	"github.com/PageWrightCloud/pagewright/storage/internal/storage"
	"github.com/PageWrightCloud/pagewright/storage/internal/storage/nfs"
)

func main() {
	cfg := config.LoadConfig()

	// Initialize storage backend
	var backend storage.Backend
	var err error

	switch cfg.StorageBackend {
	case "nfs":
		backend, err = nfs.NewNFSBackend(cfg.NFSBasePath)
		if err != nil {
			log.Fatalf("Failed to initialize NFS backend: %v", err)
		}
	default:
		log.Fatalf("Unsupported storage backend: %s", cfg.StorageBackend)
	}

	// Create API handler
	handler := api.NewHandler(backend)
	router := handler.SetupRoutes()

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Storage service starting on port %d with backend: %s", cfg.Port, cfg.StorageBackend)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}
