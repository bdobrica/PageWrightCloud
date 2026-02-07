package kubernetes

import (
	"context"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestNewKubernetesSpawner(t *testing.T) {
	spawner := NewKubernetesSpawner("test-image:latest", "default")
	assert.NotNil(t, spawner)
	assert.Equal(t, "test-image:latest", spawner.image)
	assert.Equal(t, "default", spawner.namespace)
}

func TestKubernetesSpawner_Spawn(t *testing.T) {
	spawner := NewKubernetesSpawner("test-image:latest", "default")

	job := &types.Job{
		JobID:         "job-123",
		SiteID:        "site-456",
		Prompt:        "Test prompt",
		SourceVersion: "v1",
		TargetVersion: "v2",
		FencingToken:  42,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	ctx := context.Background()
	workerID, err := spawner.Spawn(ctx, job, "http://manager:8081")

	assert.NoError(t, err)
	assert.NotEmpty(t, workerID)
}

func TestKubernetesSpawner_Close(t *testing.T) {
	spawner := NewKubernetesSpawner("test-image:latest", "default")
	err := spawner.Close()
	assert.NoError(t, err)
}
