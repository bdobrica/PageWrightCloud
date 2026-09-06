package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/artifact"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/codex"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/config"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/server"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/storage"
	"github.com/bdobrica/PageWrightCloud/pagewright/worker/internal/types"
)

func main() {
	cfg := config.LoadConfig()

	// Parse job from environment
	if cfg.JobJSON == "" {
		fmt.Println("ERROR: PAGEWRIGHT_JOB environment variable is required")
		os.Exit(1)
	}

	var job types.Job
	if err := json.Unmarshal([]byte(cfg.JobJSON), &job); err != nil {
		fmt.Printf("ERROR: Failed to parse job JSON: %v\n", err)
		os.Exit(1)
	}
	if err := job.ValidateLaunch(); err != nil {
		fmt.Printf("ERROR: Invalid job: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Worker starting for job %s (site: %s)\n", job.JobID, job.SiteID)

	// Initialize components
	storageClient := storage.NewClient(cfg.StorageURL)
	executor := codex.NewExecutor(cfg.CodexBinary, filepath.Join(cfg.WorkDir, "site"), cfg.LLMKey, cfg.LLMBaseURL)
	srv := server.NewServer(cfg.Port, executor)

	// Start HTTP server in background
	go func() {
		if err := srv.Start(); err != nil {
			fmt.Printf("ERROR: HTTP server failed: %v\n", err)
		}
	}()

	// Run the job
	if err := runJob(cfg, &job, storageClient, executor, srv); err != nil {
		fmt.Printf("ERROR: Job failed: %v\n", err)
		srv.SetError(err)

		// Report failure to manager
		reportResult(cfg.ManagerURL, &job, "failed", "", err.Error())
		os.Exit(1)
	}

	fmt.Println("Job completed successfully")
	os.Exit(0)
}

func runJob(cfg *config.Config, job *types.Job, storageClient *storage.Client, executor *codex.Executor, srv *server.Server) error {
	ctx := context.Background()

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

	// Step 4: Execute codex
	srv.UpdateStatus("executing", "Running codex exec", 40)
	fmt.Println("Executing non-interactive Codex")

	if err := executor.Execute(ctx, job.Prompt); err != nil {
		return fmt.Errorf("codex execution failed: %w", err)
	}

	// Step 5: Parse codex output
	srv.UpdateStatus("processing", "Parsing codex output", 70)
	filesChanged, summary := executor.ParseOutput()
	fmt.Printf("Files changed: %v\n", filesChanged)
	fmt.Printf("Summary: %s\n", summary)

	// Step 6: Pack result
	srv.UpdateStatus("packing", "Creating new artifact", 80)
	outputArtifact := filepath.Join(cfg.WorkDir, "output.tar.gz")

	fmt.Println("Packing result...")
	if err := artifact.Pack(siteDir, outputArtifact); err != nil {
		return fmt.Errorf("failed to pack artifact: %w", err)
	}

	// Step 7: Create manifest
	layout, err := artifact.Inspect(outputArtifact)
	if err != nil {
		return fmt.Errorf("failed to inspect packed artifact: %w", err)
	}
	entrypoints := []string{}
	if layout.Kind == "compiled" {
		entrypoints = []string{"index.html"}
	}

	manifest := types.Manifest{
		ArchiveSchemaVersion: layout.SchemaVersion,
		Kind:                 layout.Kind,
		ThemeID:              layout.ThemeID,
		SiteID:               job.SiteID,
		BuildID:              job.TargetVersion,
		BaseBuildID:          job.SourceVersion,
		FencingToken:         job.FencingToken,
		Prompt:               job.Prompt,
		CreatedAt:            time.Now().UTC(),
		FileCount:            layout.FileCount,
		TotalSize:            layout.TotalSize,
		Entrypoints:          entrypoints,
		Screenshots:          []string{}, // Stubbed for now
		ChecksPassed:         false,      // Compiler/output checks are not integrated yet.
		ConsoleErrors:        0,          // Stubbed for now
		FilesChanged:         filesChanged,
		ChangesSummary:       summary,
	}

	// Step 8: Upload artifact and manifest
	srv.UpdateStatus("uploading", "Uploading results to storage", 90)
	if err := persistAndReport(storageClient, cfg.ManagerURL, job, outputArtifact, manifest, executor.GetOutput()); err != nil {
		return err
	}
	srv.UpdateStatus("done", "Job completed", 100)
	return nil
}

func persistAndReport(storageClient *storage.Client, managerURL string, job *types.Job, outputArtifact string, manifest types.Manifest, logContent string) error {
	fmt.Printf("Uploading artifact: site=%s, version=%s\n", job.SiteID, job.TargetVersion)
	if err := storageClient.UploadArtifact(job.SiteID, job.TargetVersion, outputArtifact); err != nil {
		return fmt.Errorf("failed to upload artifact: %w", err)
	}

	// Private logs are required. The manifest is uploaded last as the storage
	// commit record; no completed callback is sent on any persistence failure.
	if err := storageClient.UploadLog(job.SiteID, job.TargetVersion, logContent); err != nil {
		return fmt.Errorf("failed to upload logs: %w", err)
	}
	fmt.Println("Committing manifest...")
	if err := storageClient.UploadManifest(job.SiteID, job.TargetVersion, manifest); err != nil {
		return fmt.Errorf("failed to upload manifest: %w", err)
	}

	// Step 9: Report completion to manager
	manifestPath := fmt.Sprintf("/sites/%s/artifacts/%s/manifest", job.SiteID, job.TargetVersion)

	if err := reportResult(managerURL, job, "completed", manifestPath, ""); err != nil {
		return fmt.Errorf("failed to report result: %w", err)
	}

	return nil
}

func reportResult(managerURL string, job *types.Job, status, manifestPath, errorMsg string) error {
	if job == nil {
		return fmt.Errorf("job is required")
	}
	result := types.JobResult{
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

	jsonData, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	// POST to manager's /jobs/{job_id}/result endpoint
	url := fmt.Sprintf("%s/jobs/%s/result", strings.TrimRight(managerURL, "/"), job.JobID)
	fmt.Printf("Reporting result to manager: %s\n", url)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to post result: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("manager returned status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Println("Result reported successfully")
	return nil
}
