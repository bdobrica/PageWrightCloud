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
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/dispatcher"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/lock"
	lockRedis "github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/lock/redis"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/queue"
	queueRedis "github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/queue/redis"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/reconciler"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner/docker"
	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/spawner/kubernetes"
)

func main() {
	cfg := config.LoadConfig()
	if cfg.LockTTL < time.Second || cfg.LockRenewInterval <= 0 || cfg.LockRenewInterval > cfg.LockTTL/3 || cfg.WorkerTimeout < cfg.LockTTL {
		log.Fatal("Require lock TTL >= 1s, renewal interval <= TTL/3, and worker timeout >= TTL")
	}
	if cfg.DispatchConcurrency < 1 || cfg.DispatchConcurrency > 128 || cfg.DispatchClaimTTL < time.Second || cfg.DispatchClaimTTL > time.Minute {
		log.Fatal("Invalid dispatch concurrency or claim TTL")
	}

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
		workerSpawner, err = docker.NewDockerSpawner(docker.Config{Image: cfg.WorkerImage, Network: cfg.WorkerNetwork, Socket: cfg.DockerSocket, WorkDir: cfg.WorkerWorkDir, StorageURL: cfg.WorkerStorageURL, LLMURL: cfg.WorkerLLMURL, LLMKey: cfg.WorkerLLMKey, AppArmorProfile: cfg.WorkerAppArmorProfile})
		if err != nil {
			log.Fatalf("Invalid Docker worker configuration: %v", err)
		}
	case "kubernetes":
		workerSpawner = kubernetes.NewKubernetesSpawner(cfg.WorkerImage, "default")
	default:
		if factory, ok := testSpawners[cfg.WorkerSpawner]; ok {
			workerSpawner = factory()
		} else {
			log.Fatalf("Unsupported worker spawner: %s", cfg.WorkerSpawner)
		}
	}
	defer workerSpawner.Close()

	// Determine manager URL for worker callbacks
	managerURL := fmt.Sprintf("http://localhost:%d", cfg.Port)
	if envURL := os.Getenv("PAGEWRIGHT_MANAGER_URL"); envURL != "" {
		managerURL = envURL
	}

	// Create API handler
	handler := api.NewHandler(queueBackend, lockMgr)
	router := handler.SetupRoutes()
	dispatchQueue := queueBackend.(queue.DispatchBackend)
	initialization, endInitialization := context.WithTimeout(context.Background(), 60*time.Second)
	if _, testOnly := testSpawners[cfg.WorkerSpawner]; !testOnly {
		if err := queueBackend.(*queueRedis.RedisBackend).ValidateDurability(initialization); err != nil {
			log.Fatalf("Unsafe Redis persistence: %v", err)
		}
	}
	if err := queueBackend.(*queueRedis.RedisBackend).ProtectReservations(initialization); err != nil {
		log.Fatalf("Reservation migration failed: %v", err)
	}
	if err := dispatchQueue.InitializeDispatch(initialization, cfg.DispatchConcurrency); err != nil {
		log.Fatalf("Queue initialization failed: %v", err)
	}
	endInitialization()
	dispatcherContext, stopDispatch := context.WithCancel(context.Background())
	dispatcherDone := make(chan struct{})
	leasesDone := make(chan struct{})
	historyDone := make(chan struct{})
	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		if cleaner, ok := workerSpawner.(spawner.Cleaner); ok {
			queueBackend.(*queueRedis.RedisBackend).MaintainWorkerCleanup(dispatcherContext, cleaner)
		}
	}()
	go func() {
		defer close(historyDone)
		queueBackend.(*queueRedis.RedisBackend).MaintainHistory(dispatcherContext)
	}()
	recoveryDone := make(chan struct{})
	go func() {
		defer close(recoveryDone)
		if inspector, ok := workerSpawner.(spawner.Inspector); ok {
			recovery := &reconciler.Reconciler{Queue: queueBackend.(*queueRedis.RedisBackend), Workers: inspector, StorageURL: cfg.WorkerStorageURL, Lifetime: cfg.WorkerTimeout}
			recovery.Run(dispatcherContext)
		}
	}()
	go func() {
		defer close(leasesDone)
		queueBackend.(*queueRedis.RedisBackend).MaintainLeases(dispatcherContext, cfg.LockTTL, cfg.LockRenewInterval, cfg.WorkerTimeout)
	}()
	dispatch := &dispatcher.Dispatcher{Queue: dispatchQueue, Locks: lockMgr, Spawner: workerSpawner, ManagerURL: managerURL, Limit: cfg.DispatchConcurrency, Lease: cfg.DispatchClaimTTL, LockTTL: cfg.LockTTL}
	go func() { defer close(dispatcherDone); dispatch.Run(dispatcherContext) }()

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
	stopDispatch()
	<-leasesDone
	<-historyDone
	<-cleanupDone
	<-recoveryDone
	select {
	case <-dispatcherDone:
	case <-time.After(45 * time.Second):
		log.Println("Dispatch shutdown timed out; persisted intent will not be relaunched")
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}

// Empty in production; integration builds register a manual launch bridge.
var testSpawners = map[string]func() spawner.Spawner{}
