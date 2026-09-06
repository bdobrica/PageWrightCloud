package supervisor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Run owns both process groups. Either child's exit tears down its sibling;
// container restart, not an unbounded inner restart loop, restores the pair.
func Run(ctx context.Context, commands [][]string) error {
	children := []*exec.Cmd{}
	exited := make(chan error, len(commands))
	startErr := error(nil)
	for _, args := range commands {
		if len(args) == 0 {
			startErr = fmt.Errorf("empty child command")
			break
		}
		child := exec.Command(args[0], args[1:]...)
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := child.Start(); err != nil {
			startErr = err
			break
		}
		children = append(children, child)
		go func() { exited <- child.Wait() }()
	}
	remaining := len(children)
	if startErr == nil {
		select {
		case <-ctx.Done():
		case err := <-exited:
			remaining--
			startErr = fmt.Errorf("supervised child exited: %v", err)
		}
	}
	for _, child := range children {
		_ = syscall.Kill(-child.Process.Pid, syscall.SIGTERM)
	}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for remaining > 0 {
		select {
		case <-exited:
			remaining--
		case <-timer.C:
			for _, child := range children {
				_ = syscall.Kill(-child.Process.Pid, syscall.SIGKILL)
			}
			for remaining > 0 {
				<-exited
				remaining--
			}
		}
	}
	return startErr
}
