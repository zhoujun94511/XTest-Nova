package legacyexec

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
)

var ErrBackgroundLimit = errors.New("background shell concurrency limit reached")

// Background starts legacy shell commands while retaining enough ownership to
// reap child processes. The original ATX implementation did not call Wait,
// which leaked process resources after short-lived commands exited.
type Background struct {
	slots chan struct{}
	mu    sync.RWMutex
	runs  map[int]*backgroundProcess
}

type backgroundProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func NewBackground(limit int) *Background {
	if limit < 1 {
		limit = 1
	}
	return &Background{slots: make(chan struct{}, limit), runs: make(map[int]*backgroundProcess)}
}

func (b *Background) Start(command string) (int, error) {
	select {
	case b.slots <- struct{}{}:
	default:
		return 0, ErrBackgroundLimit
	}

	cmd := shellCommand(command)
	prepareBackgroundCommand(cmd)
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		<-b.slots
		return 0, err
	}
	process := &backgroundProcess{cmd: cmd, done: make(chan struct{})}
	b.mu.Lock()
	b.runs[cmd.Process.Pid] = process
	b.mu.Unlock()
	go func() {
		_ = cmd.Wait()
		b.mu.Lock()
		delete(b.runs, cmd.Process.Pid)
		b.mu.Unlock()
		<-b.slots
		close(process.done)
	}()
	return cmd.Process.Pid, nil
}

func (b *Background) Running() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.runs)
}

// StopAll terminates every command still owned by the Agent and waits for the
// reaper goroutines. It is safe to call repeatedly during HTTP and process
// shutdown.
func (b *Background) StopAll(ctx context.Context) error {
	b.mu.RLock()
	runs := make([]*backgroundProcess, 0, len(b.runs))
	for _, process := range b.runs {
		runs = append(runs, process)
	}
	b.mu.RUnlock()

	var stopErrors []error
	for _, process := range runs {
		if err := stopBackgroundCommand(process.cmd); err != nil && !errors.Is(err, os.ErrProcessDone) {
			stopErrors = append(stopErrors, err)
		}
	}
	for _, process := range runs {
		select {
		case <-process.done:
		case <-ctx.Done():
			stopErrors = append(stopErrors, ctx.Err())
			return errors.Join(stopErrors...)
		}
	}
	return errors.Join(stopErrors...)
}
