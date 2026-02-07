package docker

import (
	"context"
	"testing"
	"time"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestNewDockerSpawner(t *testing.T) {
	spawner := NewDockerSpawner("test-image:latest")
	assert.NotNil(t, spawner)
	assert.Equal(t, "test-image:latest", spawner.image)
}

func TestDockerSpawner_Spawn(t *testing.T) {
	spawner := NewDockerSpawner("test-image:latest")

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

func TestDockerSpawner_Close(t *testing.T) {
	spawner := NewDockerSpawner("test-image:latest")
	err := spawner.Close()
	assert.NoError(t, err)
}
