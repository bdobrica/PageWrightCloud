package kubernetes

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/manager/internal/types"
)

func TestStubDoesNotPrintPrivateJobOrConnection(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = previous; reader.Close(); writer.Close() }()
	_, err = NewKubernetesSpawner("test", "test").Spawn(context.Background(), &types.Job{JobID: "job", Prompt: "private-prompt-sentinel"}, "http://user:private-credential-sentinel@host")
	writer.Close()
	os.Stdout = previous
	raw, _ := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private-prompt-sentinel") || strings.Contains(string(raw), "private-credential-sentinel") {
		t.Fatal("private spawn data logged")
	}
}
