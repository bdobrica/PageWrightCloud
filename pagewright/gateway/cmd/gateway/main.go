package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/auth"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/config"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/handlers"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/websocket"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Connect to database
	db, err := database.NewDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Run migrations
	migrationCtx, cancelMigrations := context.WithTimeout(context.Background(), 60*time.Second)
	err = db.RunMigrations(migrationCtx)
	cancelMigrations()
	if err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize service clients
	storageClient := clients.NewStorageClient(cfg.StorageURL)
	managerClient := clients.NewManagerClient(cfg.ManagerURL)
	servingClient := clients.NewServingClient(cfg.ServingURL)
	llmClient := clients.NewLLMClient(cfg.LLMKey, cfg.LLMURL)

	// Initialize WebSocket hub
	wsHub := websocket.NewHub()
	go wsHub.Run()

	// Initialize auth manager
	jwtManager := auth.NewJWTManager(cfg.JWTSecret, cfg.JWTExpiration)
	oauthManager := auth.NewOAuthManager(
		cfg.GoogleClientID,
		cfg.GoogleClientSecret,
		cfg.GoogleRedirectURL,
	)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(db, jwtManager, oauthManager)
	sitesHandler := handlers.NewSitesHandler(db, servingClient, cfg.DefaultPageSize)
	aliasesHandler := handlers.NewAliasesHandler(db, servingClient)
	versionsHandler := handlers.NewVersionsHandler(db, storageClient, servingClient, cfg.DefaultPageSize)
	buildHandler := handlers.NewBuildHandler(db, llmClient, managerClient)
	wsHandler := handlers.NewWebSocketHandler(wsHub)

	// Setup router
	r := mux.NewRouter()

	// Apply CORS middleware
	r.Use(middleware.CORS)

	// Public routes
	r.HandleFunc("/auth/register", authHandler.Register).Methods("POST", "OPTIONS")
	r.HandleFunc("/auth/login", authHandler.Login).Methods("POST", "OPTIONS")
	r.HandleFunc("/auth/forgot-password", authHandler.ForgotPassword).Methods("POST", "OPTIONS")
	r.HandleFunc("/auth/reset-password", authHandler.ResetPassword).Methods("POST", "OPTIONS")
	r.HandleFunc("/auth/google/login", authHandler.GoogleLogin).Methods("GET", "OPTIONS")
	r.HandleFunc("/auth/google/callback", authHandler.GoogleCallback).Methods("GET", "OPTIONS")

	// Protected routes
	api := r.PathPrefix("/").Subrouter()
	api.Use(middleware.AuthMiddleware(jwtManager))

	// Auth
	api.HandleFunc("/auth/update-password", authHandler.UpdatePassword).Methods("POST", "OPTIONS")

	// WebSocket
	api.HandleFunc("/ws", wsHandler.HandleWebSocket).Methods("GET", "OPTIONS")

	// Sites
	api.HandleFunc("/sites", sitesHandler.CreateSite).Methods("POST", "OPTIONS")
	api.HandleFunc("/sites", sitesHandler.ListSites).Methods("GET", "OPTIONS")
	api.HandleFunc("/sites/{fqdn}", sitesHandler.GetSite).Methods("GET", "OPTIONS")
	api.HandleFunc("/sites/{fqdn}", sitesHandler.DeleteSite).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/sites/{fqdn}/enable", sitesHandler.EnableSite).Methods("POST", "OPTIONS")
	api.HandleFunc("/sites/{fqdn}/disable", sitesHandler.DisableSite).Methods("POST", "OPTIONS")

	// Aliases
	api.HandleFunc("/sites/{fqdn}/aliases", aliasesHandler.ListAliases).Methods("GET", "OPTIONS")
	api.HandleFunc("/sites/{fqdn}/aliases", aliasesHandler.AddAlias).Methods("POST", "OPTIONS")
	api.HandleFunc("/sites/{fqdn}/aliases/{alias}", aliasesHandler.DeleteAlias).Methods("DELETE", "OPTIONS")

	// Versions
	api.HandleFunc("/sites/{fqdn}/versions", versionsHandler.ListVersions).Methods("GET", "OPTIONS")
	api.HandleFunc("/sites/{fqdn}/versions/{version_id}/deploy", versionsHandler.DeployVersion).Methods("POST", "OPTIONS")
	api.HandleFunc("/sites/{fqdn}/versions/{version_id}", versionsHandler.DeleteVersion).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/sites/{fqdn}/versions/{version_id}/download", versionsHandler.DownloadVersion).Methods("GET", "OPTIONS")

	// Build (chat interface)
	api.HandleFunc("/sites/{fqdn}/build", buildHandler.Build).Methods("POST", "OPTIONS")

	// Health check
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods("GET")

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("BFF server starting on %s", addr)

	server := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
