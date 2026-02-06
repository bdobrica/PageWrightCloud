package docker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/google/uuid"
)

type DockerSpawner struct {
	image string
}

func NewDockerSpawner(image string) *DockerSpawner {
	return &DockerSpawner{
		image: image,
	}
}

func (d *DockerSpawner) Spawn(ctx context.Context, job *types.Job, managerURL string) (string, error) {
	workerID := uuid.New().String()

	// Marshal job to JSON
	jobJSON, err := json.Marshal(job)
	if err != nil {
		return "", fmt.Errorf("failed to marshal job: %w", err)
	}

	// For PoC, we'll just log the spawn request
	// In production, this would use Docker API to spawn a container
	fmt.Printf("DOCKER SPAWN: Would spawn container:\n")
	fmt.Printf("  Image: %s\n", d.image)
	fmt.Printf("  Worker ID: %s\n", workerID)
	fmt.Printf("  Job: %s\n", string(jobJSON))
	fmt.Printf("  Manager URL: %s\n", managerURL)
	fmt.Printf("  Env: PAGEWRIGHT_JOB=%s\n", string(jobJSON))
	fmt.Printf("  Env: PAGEWRIGHT_MANAGER_URL=%s\n", managerURL)
	fmt.Printf("  Env: PAGEWRIGHT_WORKER_ID=%s\n", workerID)

	// TODO: Actually spawn docker container:
	// docker run -e PAGEWRIGHT_JOB=<json> -e PAGEWRIGHT_MANAGER_URL=<url> -e PAGEWRIGHT_WORKER_ID=<id> <image>

	return workerID, nil
}

func (d *DockerSpawner) Close() error {
	return nil
}
