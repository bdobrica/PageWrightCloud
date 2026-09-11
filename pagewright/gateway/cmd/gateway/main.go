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

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/auth"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/clients"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/config"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/handlers"
	resetmail "github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/mail"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/pilot"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/runtimehttp"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/serviceauth"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

func main() {
	if err := serviceauth.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// Load configuration
	cfg := config.LoadConfig()
	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}
	originPolicy, err := middleware.OriginPolicy(cfg.AppOrigins, cfg.SiteDomain)
	if err != nil {
		log.Fatal(err)
	}
	limits, err := config.LoadPilot()
	if err != nil {
		log.Fatal(err)
	}
	resetSender, err := resetmail.FromEnv(cfg.AppOrigins)
	if err != nil {
		log.Fatal(err)
	}

	// Connect to database
	db, err := database.NewDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Failed to connect to database; verify PostgreSQL configuration and readiness")
	}
	defer db.Close()

	// Run migrations
	migrationCtx, cancelMigrations := context.WithTimeout(context.Background(), 60*time.Second)
	err = db.RunMigrations(migrationCtx)
	cancelMigrations()
	if err != nil {
		log.Fatal("Failed to run migrations; operator database inspection required")
	}

	// Initialize service clients
	storageClient := clients.NewStorageClient(cfg.StorageURL)
	managerClient := clients.NewManagerClient(cfg.ManagerURL)
	servingClient := clients.NewServingClient(cfg.ServingURL)
	provider, err := pilot.NewProvider(db, limits.ProxyToken, cfg.LLMKey, cfg.LLMURL, limits.BudgetCents, limits.Active, limits.DevelopmentSignup)
	if err != nil {
		log.Fatal(err)
	}
	llmClient := clients.NewLLMClient(limits.ProxyToken, "http://127.0.0.1:8087/v1")

	// Initialize auth manager
	jwtManager := auth.NewJWTManager(cfg.JWTSecret, cfg.JWTExpiration)
	oauthManager := auth.NewOAuthManager(
		cfg.GoogleClientID,
		cfg.GoogleClientSecret,
		cfg.GoogleRedirectURL,
	)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(db, jwtManager, oauthManager)
	if resetSender != nil {
		authHandler.SetResetSender(resetSender)
	}
	sitesHandler := handlers.NewSitesHandler(db, servingClient, storageClient, cfg.DefaultPageSize)
	sitesHandler.SetHostingAddress(cfg.HostingScheme, cfg.HostingPort)
	sitesHandler.SetTLSStatePath(cfg.TLSStatePath)
	sitesHandler.RegistrationOpen = limits.DevelopmentSignup
	if err := sitesHandler.SetSiteDomain(cfg.SiteDomain); err != nil {
		log.Fatal(err)
	}
	aliasesHandler := handlers.NewAliasesHandler(db, servingClient)
	versionsHandler := handlers.NewVersionsHandler(db, storageClient, servingClient, cfg.DefaultPageSize)
	versionsHandler.SetHostingAddress(cfg.HostingScheme, cfg.HostingPort)
	versionsHandler.SetTLSStatePath(cfg.TLSStatePath)
	buildHandler := handlers.NewBuildHandler(db, llmClient, managerClient, storageClient)
	buildHandler.SetPilotLimits(db, database.PilotLimits{UserDaily: limits.UserDaily, SiteDaily: limits.SiteDaily, Active: limits.Active})
	buildHandler.SetPilotAIAllowance(limits.BudgetCents)
	recoveryContext, stopRecovery := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopRecovery()
	recoveryDone := make(chan struct{})
	go func() { defer close(recoveryDone); buildHandler.RunRecovery(recoveryContext) }()
	deploymentRecoveryDone := make(chan struct{})
	go func() { defer close(deploymentRecoveryDone); versionsHandler.RunDeploymentRecovery(recoveryContext) }()

	// Setup router
	r := mux.NewRouter()
	r.HandleFunc("/ready", runtimehttp.Ready(db.PingContext,
		runtimehttp.HTTP(cfg.StorageURL+"/ready"), runtimehttp.HTTP(cfg.ManagerURL+"/ready"),
		runtimehttp.HTTP(cfg.ServingURL+"/ready")))

	// The server's outer origin policy runs before router throttling and auth.
	r.Use(middleware.PilotThrottle(db, false))

	// Public routes
	// Explicit retirement response; never accepts credentials or upgrades.
	r.HandleFunc("/ws", handlers.WebSocketDisabled)
	r.HandleFunc("/auth/register", middleware.RegistrationGate(limits.DevelopmentSignup, authHandler.Register)).Methods("POST", "OPTIONS")
	r.HandleFunc("/auth/login", authHandler.Login).Methods("POST", "OPTIONS")
	r.HandleFunc("/auth/forgot-password", authHandler.ForgotPassword).Methods("POST", "OPTIONS")
	r.HandleFunc("/auth/reset-password", authHandler.ResetPassword).Methods("POST", "OPTIONS")
	r.HandleFunc("/auth/google/login", authHandler.GoogleLogin).Methods("GET", "OPTIONS")
	r.HandleFunc("/auth/google/callback", authHandler.GoogleCallback).Methods("GET", "OPTIONS")

	// Protected routes
	api := r.PathPrefix("/").Subrouter()
	api.Use(middleware.AuthMiddleware(jwtManager))
	api.Use(middleware.PilotThrottle(db, true))

	// Auth
	api.HandleFunc("/auth/update-password", authHandler.UpdatePassword).Methods("POST", "OPTIONS")

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
	api.HandleFunc("/sites/{fqdn}/deployment", versionsHandler.DeploymentStatus).Methods("GET", "OPTIONS")
	api.HandleFunc("/sites/{fqdn}/versions/{version_id}/deploy", versionsHandler.DeployVersion).Methods("POST", "OPTIONS")
	api.HandleFunc("/sites/{fqdn}/versions/{version_id}", versionsHandler.DeleteVersion).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/sites/{fqdn}/versions/{version_id}/download", versionsHandler.DownloadVersion).Methods("GET", "OPTIONS")

	// Build (chat interface)
	api.HandleFunc("/sites/{fqdn}/build", buildHandler.Build).Methods("POST", "OPTIONS")
	api.HandleFunc("/sites/{fqdn}/jobs", buildHandler.Jobs).Methods("GET", "OPTIONS")
	api.HandleFunc("/sites/{fqdn}/jobs/{job_id}", buildHandler.Jobs).Methods("GET", "OPTIONS")

	// Public MVP configuration (OPTIONS is needed by the cross-origin UI).
	r.HandleFunc("/capabilities", sitesHandler.Capabilities).Methods("GET", "OPTIONS")
	// Health check
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods("GET")

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("BFF server starting on %s", addr)

	server := &http.Server{
		Addr:              addr,
		Handler:           runtimehttp.Budget(55*time.Second, originPolicy(r)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      65 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	providerServer := &http.Server{Addr: ":8087", Handler: provider, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 150 * time.Second, IdleTimeout: 30 * time.Second}
	go func() {
		if err := providerServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("internal provider listener failed")
		}
	}()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-recoveryContext.Done():
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(recoveryContext, 5*time.Second)
				_ = db.PrunePilotRates(ctx)
				_ = db.PrunePasswordResetTokens(ctx)
				cancel()
			}
		}
	}()

	go func() {
		<-recoveryContext.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
		_ = providerServer.Shutdown(shutdown)
	}()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
	stopRecovery()
	<-recoveryDone
	<-deploymentRecoveryDone
}
