package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/PageWrightCloud/pagewright/manager/internal/types"
)

func main() {
	// Get job from environment
	jobJSON := os.Getenv("PAGEWRIGHT_JOB")
	if jobJSON == "" {
		log.Fatal("PAGEWRIGHT_JOB environment variable not set")
	}

	managerURL := os.Getenv("PAGEWRIGHT_MANAGER_URL")
	if managerURL == "" {
		log.Fatal("PAGEWRIGHT_MANAGER_URL environment variable not set")
	}

	workerID := os.Getenv("PAGEWRIGHT_WORKER_ID")
	if workerID == "" {
		log.Fatal("PAGEWRIGHT_WORKER_ID environment variable not set")
	}

	// Parse job
	var job types.Job
	if err := json.Unmarshal([]byte(jobJSON), &job); err != nil {
		log.Fatalf("Failed to parse job: %v", err)
	}

	log.Printf("Worker %s starting for job %s", workerID, job.JobID)
	log.Printf("Site ID: %s", job.SiteID)
	log.Printf("Prompt: %s", job.Prompt)
	log.Printf("Source Version: %s", job.SourceVersion)
	log.Printf("Target Version: %s", job.TargetVersion)
	log.Printf("Fencing Token: %d", job.FencingToken)

	// Simulate work
	log.Println("Processing job...")
	time.Sleep(2 * time.Second)

	// Generate result
	result := fmt.Sprintf("Stub worker processed prompt: '%s' for site '%s'. Target version: %s",
		job.Prompt, job.SiteID, job.TargetVersion)

	log.Println("Job completed, sending callback to manager...")

	// Call back to manager
	statusUpdate := types.JobStatusUpdate{
		Status: types.JobStatusCompleted,
		Result: result,
	}

	if err := sendCallback(managerURL, job.JobID, statusUpdate); err != nil {
		log.Fatalf("Failed to send callback: %v", err)
	}

	log.Println("Callback sent successfully, worker exiting")
}

func sendCallback(managerURL, jobID string, update types.JobStatusUpdate) error {
	url := fmt.Sprintf("%s/jobs/%s/status", managerURL, jobID)

	jsonData, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("failed to marshal update: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send callback: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("callback failed with status: %d", resp.StatusCode)
	}

	return nil
}
