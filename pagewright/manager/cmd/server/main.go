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

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/api"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/config"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/lock"
	lockRedis "github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/lock/redis"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/queue"
	queueRedis "github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/queue/redis"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner/docker"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner/kubernetes"
)

func main() {
	cfg := config.LoadConfig()

	// Initialize queue backend
	var queueBackend queue.Backend
	var err error

	switch cfg.QueueBackend {
	case "redis":
		queueBackend, err = queueRedis.NewRedisBackend(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
		if err != nil {
			log.Fatalf("Failed to initialize Redis queue backend: %v", err)
		}
	default:
		log.Fatalf("Unsupported queue backend: %s", cfg.QueueBackend)
	}
	defer queueBackend.Close()

	// Initialize lock manager
	var lockMgr lock.Manager

	switch cfg.QueueBackend {
	case "redis":
		lockMgr, err = lockRedis.NewRedisLockManager(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
		if err != nil {
			log.Fatalf("Failed to initialize Redis lock manager: %v", err)
		}
	default:
		log.Fatalf("Unsupported lock backend: %s", cfg.QueueBackend)
	}
	defer lockMgr.Close()

	// Initialize worker spawner
	var workerSpawner spawner.Spawner

	switch cfg.WorkerSpawner {
	case "docker":
		workerSpawner = docker.NewDockerSpawner(cfg.WorkerImage)
	case "kubernetes":
		workerSpawner = kubernetes.NewKubernetesSpawner(cfg.WorkerImage, "default")
	default:
		log.Fatalf("Unsupported worker spawner: %s", cfg.WorkerSpawner)
	}
	defer workerSpawner.Close()

	// Determine manager URL for worker callbacks
	managerURL := fmt.Sprintf("http://localhost:%d", cfg.Port)
	if envURL := os.Getenv("PAGEWRIGHT_MANAGER_URL"); envURL != "" {
		managerURL = envURL
	}

	// Create API handler
	handler := api.NewHandler(queueBackend, lockMgr, workerSpawner, cfg.LockTTL, managerURL)
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
		log.Printf("Manager service starting on port %d", cfg.Port)
		log.Printf("Queue backend: %s", cfg.QueueBackend)
		log.Printf("Worker spawner: %s", cfg.WorkerSpawner)
		log.Printf("Manager URL: %s", managerURL)
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
