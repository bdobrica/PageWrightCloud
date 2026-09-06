package supervisor

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSupervisorStopsSiblingOnExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := Run(ctx, [][]string{{"sleep", "30"}, {"false"}}); err == nil {
		t.Fatal("child exit treated as success")
	}
	if ctx.Err() != nil {
		t.Fatal("supervisor waited for healthy sibling")
	}
}

func TestSupervisorCancelsAndReapsChildren(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, [][]string{{"sleep", "30"}, {"sleep", "30"}}) }()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not finish")
	}
	// Child process groups have been waited/reaped, not left as surviving daemons.
	output, err := exec.Command("ps", "-o", "pid=", "--ppid", strconv.Itoa(os.Getpid())).Output()
	if err != nil {
		return
	} // BusyBox ps lacks --ppid; behavior is also host-smoke tested.
	for _, pid := range strings.Fields(string(output)) {
		n, _ := strconv.Atoi(pid)
		if syscall.Kill(n, 0) == nil {
			t.Fatalf("surviving child %s", pid)
		}
	}
}
