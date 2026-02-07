package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/google/uuid"
)

type KubernetesSpawner struct {
	image     string
	namespace string
}

func NewKubernetesSpawner(image, namespace string) *KubernetesSpawner {
	return &KubernetesSpawner{
		image:     image,
		namespace: namespace,
	}
}

func (k *KubernetesSpawner) Spawn(ctx context.Context, job *types.Job, managerURL string) (string, error) {
	workerID := uuid.New().String()

	// Marshal job to JSON
	jobJSON, err := json.Marshal(job)
	if err != nil {
		return "", fmt.Errorf("failed to marshal job: %w", err)
	}

	// For PoC, we'll just log the spawn request
	// In production, this would use Kubernetes API to create a Job or Pod
	fmt.Printf("KUBERNETES SPAWN: Would create Job:\n")
	fmt.Printf("  Namespace: %s\n", k.namespace)
	fmt.Printf("  Image: %s\n", k.image)
	fmt.Printf("  Worker ID: %s\n", workerID)
	fmt.Printf("  Job: %s\n", string(jobJSON))
	fmt.Printf("  Manager URL: %s\n", managerURL)
	fmt.Printf("  Env: PAGEWRIGHT_JOB=%s\n", string(jobJSON))
	fmt.Printf("  Env: PAGEWRIGHT_MANAGER_URL=%s\n", managerURL)
	fmt.Printf("  Env: PAGEWRIGHT_WORKER_ID=%s\n", workerID)

	// TODO: Actually create Kubernetes Job:
	// kubectl run pagewright-worker-<id> --image=<image> --env=PAGEWRIGHT_JOB=<json> ...

	return workerID, nil
}

func (k *KubernetesSpawner) Close() error {
	return nil
}
