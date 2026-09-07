package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/artifact"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/build"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/codex"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/config"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/server"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/storage"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/types"
)

func main() {
	// Last-resort whole-container ceiling, including blocked I/O and callbacks.
	// PID 1/init exits with runner, so Docker tears down remaining processes.
	watchdog := time.AfterFunc(16*time.Minute, func() { os.Exit(124) })
	defer watchdog.Stop()
	cfg := config.LoadConfig()

	// Parse job from environment
	if cfg.JobJSON == "" {
		fmt.Println("ERROR: PAGEWRIGHT_JOB environment variable is required")
		os.Exit(1)
	}

	var job types.Job
	if err := json.Unmarshal([]byte(cfg.JobJSON), &job); err != nil {
		fmt.Println("ERROR: Failed to parse job JSON; payload withheld")
		os.Exit(1)
	}
	if err := job.ValidateLaunch(); err != nil {
		fmt.Printf("ERROR: Invalid job: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Worker starting for job %s (site: %s)\n", job.JobID, job.SiteID)

	// Initialize components
	storageClient := storage.NewClient(cfg.StorageURL)
	executor := codex.NewExecutor(cfg.CodexBinary, filepath.Join(cfg.WorkDir, "site"), cfg.LLMKey, cfg.LLMBaseURL).WithModel(cfg.LLMModel)
	srv := server.NewServer(cfg.Port, executor)

	// Start HTTP server in background
	go func() {
		if err := srv.Start(); err != nil {
			fmt.Printf("ERROR: HTTP server failed: %v\n", err)
		}
	}()

	// Run the job
	if err := runJob(cfg, &job, storageClient, executor, srv); err != nil {
		fmt.Println("ERROR: Job failed; raw execution diagnostics withheld")
		srv.SetError(err)

		// Report failure to manager
		if !errors.Is(err, errDeliveryUncertain) {
			reportResult(cfg.ManagerURL, &job, "failed", "", "Worker execution failed; raw diagnostics withheld")
		}
		os.Exit(1)
	}

	fmt.Println("Job completed successfully")
	os.Exit(0)
}

func runJob(cfg *config.Config, job *types.Job, storageClient *storage.Client, executor *codex.Executor, srv *server.Server) error {
	signals, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stopSignals()
	ctx, cancel := context.WithTimeout(signals, 15*time.Minute)
	defer cancel()
	return runJobWithContext(ctx, cfg, job, storageClient, executor, srv)
}

// A caller-owned deadline permits deterministic cancellation acceptance without
// introducing a runtime flag that can weaken the production lifetime ceiling.
func runJobWithContext(parent context.Context, cfg *config.Config, job *types.Job, storageClient *storage.Client, executor *codex.Executor, srv *server.Server) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	srv.SetJobCancel(cancel)
	defer srv.SetJobCancel(nil)
	storageClient = storageClient.WithContext(ctx)

	// Step 1: Fetch artifact
	srv.UpdateStatus("fetching", "Downloading artifact from storage", 10)
	artifactPath := filepath.Join(cfg.WorkDir, "artifact.tar.gz")

	fmt.Printf("Fetching artifact: site=%s, version=%s\n", job.SiteID, job.SourceVersion)
	if err := storageClient.FetchArtifact(job.SiteID, job.SourceVersion, artifactPath); err != nil {
		return fmt.Errorf("failed to fetch artifact: %w", err)
	}

	// Step 2: Unpack artifact
	srv.UpdateStatus("unpacking", "Extracting artifact", 20)
	siteDir := filepath.Join(cfg.WorkDir, "site")

	if err := os.MkdirAll(siteDir, 0755); err != nil {
		return fmt.Errorf("failed to create site directory: %w", err)
	}

	fmt.Println("Unpacking artifact...")
	if err := artifact.Unpack(artifactPath, siteDir); err != nil {
		return fmt.Errorf("failed to unpack artifact: %w", err)
	}

	// Step 3: Patch instructions
	srv.UpdateStatus("patching", "Patching codex instructions", 30)
	fmt.Println("Patching .codex/instructions.md...")
	if err := artifact.PatchInstructions(siteDir, cfg.InstructionsPath); err != nil {
		return fmt.Errorf("failed to patch instructions: %w", err)
	}
	before, err := artifact.SnapshotWorkspace(siteDir)
	if err != nil {
		return fmt.Errorf("invalid initial workspace: %w", err)
	}

	// Step 4: Execute codex
	srv.UpdateStatus("executing", "Running codex exec", 40)
	fmt.Println("Executing non-interactive Codex")

	if err := executor.Execute(ctx, job.Prompt); err != nil {
		return fmt.Errorf("codex execution failed: %w", err)
	}

	// Step 5: Parse codex output
	srv.UpdateStatus("processing", "Parsing codex output", 70)
	_, summary := executor.ParseOutput()
	fmt.Println("Execution summary withheld from operational logs")

	// Validate the edit, freeze source and compile before packing or uploading.
	srv.UpdateStatus("compiling", "Validating source and compiling trusted theme", 80)
	outputArtifact := filepath.Join(cfg.WorkDir, "output.tar.gz")
	compiled, err := build.Compile(ctx, siteDir, cfg.CompilerBinary, cfg.ThemePath, outputArtifact, before)
	if err != nil {
		return fmt.Errorf("build validation failed: %w", err)
	}
	layout := compiled.Layout
	entrypoints := []string{}
	if layout.Kind == "compiled" {
		entrypoints = []string{"index.html"}
	}

	manifest := types.Manifest{
		ArchiveSchemaVersion:   layout.SchemaVersion,
		Kind:                   layout.Kind,
		ThemeID:                layout.ThemeID,
		SiteID:                 job.SiteID,
		BuildID:                job.TargetVersion,
		BaseBuildID:            job.SourceVersion,
		FencingToken:           job.FencingToken,
		Prompt:                 job.Prompt,
		CreatedAt:              time.Now().UTC(),
		FileCount:              layout.FileCount,
		TotalSize:              layout.TotalSize,
		Entrypoints:            entrypoints,
		Screenshots:            []string{}, // Stubbed for now
		ChecksPassed:           true,       // Only after every named static gate succeeds.
		ValidationChecks:       compiled.Checks,
		CompilerVersion:        build.CompilerVersion,
		ThemeVersion:           build.ThemeVersion,
		ConsoleErrors:          0, // Legacy count; browser_checks_performed=false means unmeasured.
		BrowserChecksPerformed: false,
		FilesChanged:           compiled.FilesChanged,
		ChangesSummary:         summary,
	}

	// Step 8: Upload artifact and manifest
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("job stopped before upload: %w", err)
	}
	srv.UpdateStatus("uploading", "Uploading results to storage", 90)
	if err := persistAndReport(storageClient, cfg.ManagerURL, job, outputArtifact, manifest, executor.GetOutput()); err != nil {
		return err
	}
	srv.UpdateStatus("done", "Job completed", 100)
	return nil
}

func persistAndReport(storageClient *storage.Client, managerURL string, job *types.Job, outputArtifact string, manifest types.Manifest, logContent string) error {
	storageClient = storageClient.WithAttempt(job)
	fmt.Printf("Uploading artifact: site=%s, version=%s\n", job.SiteID, job.TargetVersion)
	if err := storageClient.UploadArtifact(job.SiteID, job.TargetVersion, outputArtifact); err != nil {
		return fmt.Errorf("failed to upload artifact: %w", err)
	}

	// Private logs are required. The manifest is uploaded last as the storage
	// commit record; no completed callback is sent on any persistence failure.
	if err := storageClient.UploadLog(job.SiteID, job.TargetVersion, privateDiagnostic(job, logContent)); err != nil {
		return fmt.Errorf("failed to upload logs: %w", err)
	}
	fmt.Println("Committing manifest...")
	if err := storageClient.UploadManifest(job.SiteID, job.TargetVersion, manifest); err != nil {
		return fmt.Errorf("%w: manifest acknowledgement unavailable: %v", errDeliveryUncertain, err)
	}

	// Step 9: Report completion to manager
	manifestPath := fmt.Sprintf("/sites/%s/artifacts/%s/manifest", job.SiteID, job.TargetVersion)

	if err := reportResult(managerURL, job, "completed", manifestPath, ""); err != nil {
		return fmt.Errorf("%w: %v", errDeliveryUncertain, err)
	}

	return nil
}

func reportResult(managerURL string, job *types.Job, status, manifestPath, errorMsg string) error {
	if job == nil {
		return fmt.Errorf("job is required")
	}
	result := types.JobResult{
		LockToken:     job.LockToken,
		FencingToken:  job.FencingToken,
		JobID:         job.JobID,
		SiteID:        job.SiteID,
		OwnerID:       job.OwnerID,
		SourceVersion: job.SourceVersion,
		Status:        status,
		TargetVersion: job.TargetVersion,
		ManifestPath:  manifestPath,
		ErrorMessage:  errorMsg,
	}

	if status == "completed" {
		result.Result = fmt.Sprintf("Successfully processed site %s", job.SiteID)
	}
	if err := result.Validate(); err != nil {
		return fmt.Errorf("invalid result: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	return deliverResult(ctx, managerURL, result, 4, 250*time.Millisecond)
}
