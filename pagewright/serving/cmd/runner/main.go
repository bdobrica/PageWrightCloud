package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/artifact"
	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/config"
	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/nginx"
	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/server"
	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/serviceauth"
	"github.com/bdobrica/PageWrightCloud/pagewright/serving/internal/storage"
)

func main() {
	if err := serviceauth.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cfg := config.LoadConfig()
	if err := cfg.ValidatePaths(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("PageWright Serving Service starting on port %d\n", cfg.Port)
	fmt.Printf("WWW Root: %s\n", cfg.WWWRoot)
	fmt.Printf("Nginx sites-enabled: %s\n", cfg.NginxSitesEnabled)
	fmt.Printf("Max versions per site: %d\n", cfg.MaxVersionsPerSite)

	// Initialize components
	artifactMgr := artifact.NewManager(cfg.WWWRoot, cfg.MaxVersionsPerSite)
	nginxMgr := nginx.NewManager(cfg.NginxSitesEnabled, cfg.NginxReloadCommand, cfg.MaintenancePagePath)
	nginxMgr.SetLifecycle("nginx -t", "http://127.0.0.1:8089/")
	storageCli := storage.NewClient(cfg.StorageURL)

	// Create maintenance page if it doesn't exist
	if err := ensureMaintenancePage(cfg.MaintenancePagePath); err != nil {
		fmt.Printf("Warning: failed to create maintenance page: %v\n", err)
	}

	// Setup HTTP server
	handler := server.NewHandler(artifactMgr, nginxMgr, storageCli)
	router := serviceauth.Wrap("serving", handler.SetupRoutes())

	addr := fmt.Sprintf(":%d", cfg.Port)
	fmt.Printf("Server listening on %s\n", addr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Addr: addr, Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second}
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Printf("Server failed: %v\n", err)
		os.Exit(1)
	}
	stop()
	<-done
}

func ensureMaintenancePage(path string) error {
	// Check if file exists
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	// Create directory
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	// Create default maintenance page
	content := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Service Unavailable</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
        }
        .container {
            text-align: center;
            padding: 2rem;
        }
        h1 {
            font-size: 3rem;
            margin-bottom: 1rem;
        }
        p {
            font-size: 1.2rem;
            opacity: 0.9;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>503</h1>
        <p>Service Temporarily Unavailable</p>
        <p>We'll be back soon!</p>
    </div>
</body>
</html>
`

	return os.WriteFile(path, []byte(content), 0644)
}
