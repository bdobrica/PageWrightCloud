package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/PageWrightCloud/pagewright/bff/internal/auth"
	"github.com/PageWrightCloud/pagewright/bff/internal/clients"
	"github.com/PageWrightCloud/pagewright/bff/internal/config"
	"github.com/PageWrightCloud/pagewright/bff/internal/database"
	"github.com/PageWrightCloud/pagewright/bff/internal/handlers"
	"github.com/PageWrightCloud/pagewright/bff/internal/middleware"
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
	if err := runMigrations(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize service clients
	storageClient := clients.NewStorageClient(cfg.StorageURL)
	managerClient := clients.NewManagerClient(cfg.ManagerURL)
	servingClient := clients.NewServingClient(cfg.ServingURL)
	llmClient := clients.NewLLMClient(cfg.LLMKey, cfg.LLMURL)

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

	// Setup router
	r := mux.NewRouter()

	// Apply CORS middleware
	r.Use(middleware.CORS)

	// Public routes
	r.HandleFunc("/auth/register", authHandler.Register).Methods("POST")
	r.HandleFunc("/auth/login", authHandler.Login).Methods("POST")
	r.HandleFunc("/auth/google/login", authHandler.GoogleLogin).Methods("GET")
	r.HandleFunc("/auth/google/callback", authHandler.GoogleCallback).Methods("GET")

	// Protected routes
	api := r.PathPrefix("/").Subrouter()
	api.Use(middleware.AuthMiddleware(jwtManager))

	// Sites
	api.HandleFunc("/sites", sitesHandler.CreateSite).Methods("POST")
	api.HandleFunc("/sites", sitesHandler.ListSites).Methods("GET")
	api.HandleFunc("/sites/{fqdn}", sitesHandler.GetSite).Methods("GET")
	api.HandleFunc("/sites/{fqdn}", sitesHandler.DeleteSite).Methods("DELETE")
	api.HandleFunc("/sites/{fqdn}/enable", sitesHandler.EnableSite).Methods("POST")
	api.HandleFunc("/sites/{fqdn}/disable", sitesHandler.DisableSite).Methods("POST")

	// Aliases
	api.HandleFunc("/sites/{fqdn}/aliases", aliasesHandler.ListAliases).Methods("GET")
	api.HandleFunc("/sites/{fqdn}/aliases", aliasesHandler.AddAlias).Methods("POST")
	api.HandleFunc("/sites/{fqdn}/aliases/{alias}", aliasesHandler.DeleteAlias).Methods("DELETE")

	// Versions
	api.HandleFunc("/sites/{fqdn}/versions", versionsHandler.ListVersions).Methods("GET")
	api.HandleFunc("/sites/{fqdn}/versions/{version_id}/deploy", versionsHandler.DeployVersion).Methods("POST")
	api.HandleFunc("/sites/{fqdn}/versions/{version_id}", versionsHandler.DeleteVersion).Methods("DELETE")
	api.HandleFunc("/sites/{fqdn}/versions/{version_id}/download", versionsHandler.DownloadVersion).Methods("GET")

	// Build (chat interface)
	api.HandleFunc("/sites/{fqdn}/build", buildHandler.Build).Methods("POST")

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

// runMigrations runs database migrations
func runMigrations(db *database.DB) error {
	migrations := []string{
		// Check if migrations table exists
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
	}

	for _, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	// Apply versioned migrations
	migrationFiles := []struct {
		version int
		sql     string
	}{
		{1, `
			CREATE TABLE IF NOT EXISTS users (
				id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				email VARCHAR(255) NOT NULL UNIQUE,
				password_hash VARCHAR(255),
				oauth_provider VARCHAR(50),
				oauth_id VARCHAR(255),
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				UNIQUE(oauth_provider, oauth_id)
			);
			CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
			CREATE INDEX IF NOT EXISTS idx_users_oauth ON users(oauth_provider, oauth_id);
		`},
		{2, `
			CREATE TABLE IF NOT EXISTS sites (
				id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				fqdn VARCHAR(255) NOT NULL UNIQUE,
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				template_id VARCHAR(100) NOT NULL,
				live_version_id VARCHAR(100),
				preview_version_id VARCHAR(100),
				enabled BOOLEAN NOT NULL DEFAULT true,
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_sites_user_id ON sites(user_id);
			CREATE INDEX IF NOT EXISTS idx_sites_fqdn ON sites(fqdn);
		`},
		{3, `
			CREATE TABLE IF NOT EXISTS site_aliases (
				id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				site_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
				alias VARCHAR(255) NOT NULL UNIQUE,
				created_at TIMESTAMP NOT NULL DEFAULT NOW()
			);
			CREATE INDEX IF NOT EXISTS idx_site_aliases_site_id ON site_aliases(site_id);
		`},
		{4, `
			CREATE TABLE IF NOT EXISTS versions (
				id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				site_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
				build_id VARCHAR(100) NOT NULL,
				status VARCHAR(50) NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				UNIQUE(site_id, build_id)
			);
			CREATE INDEX IF NOT EXISTS idx_versions_site_id ON versions(site_id);
			CREATE INDEX IF NOT EXISTS idx_versions_created_at ON versions(created_at DESC);
		`},
	}

	for _, m := range migrationFiles {
		var applied bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", m.version).Scan(&applied)
		if err != nil {
			return fmt.Errorf("failed to check migration %d: %w", m.version, err)
		}

		if !applied {
			if _, err := db.Exec(m.sql); err != nil {
				return fmt.Errorf("failed to apply migration %d: %w", m.version, err)
			}

			if _, err := db.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", m.version); err != nil {
				return fmt.Errorf("failed to record migration %d: %w", m.version, err)
			}

			log.Printf("Applied migration %d", m.version)
		}
	}

	return nil
}
