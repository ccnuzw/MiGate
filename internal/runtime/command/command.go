package command

import (
	"bytes"
	"context"
	"os/exec"
	"syscall"
	"time"
)

const DefaultOutputLimit = 64 * 1024

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
	RunOutput(ctx context.Context, name string, args ...string) ([]byte, error)
}

type RealCommandRunner struct {
	Timeout     time.Duration
	OutputLimit int
}

type LimitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func NewLimitedBuffer(limit int) *LimitedBuffer {
	if limit <= 0 {
		limit = DefaultOutputLimit
	}
	return &LimitedBuffer{limit: limit}
}

func NewRealCommandRunner(timeout time.Duration) RealCommandRunner {
	return RealCommandRunner{Timeout: timeout, OutputLimit: DefaultOutputLimit}
}

func (r RealCommandRunner) Run(ctx context.Context, name string, args ...string) error {
	_, err := r.RunOutput(ctx, name, args...)
	return err
}

func (r RealCommandRunner) RunOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	cmd := exec.Command(name, args...)
	// Run commands in their own process group so a timeout kills child
	// processes spawned by shells (for example: sh -c "sleep 1").
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out := NewLimitedBuffer(r.outputLimit())
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		return out.Bytes(), err
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	select {
	case err := <-waitCh:
		return out.Bytes(), err
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-waitCh
		return out.Bytes(), ctx.Err()
	}
}

func (r RealCommandRunner) outputLimit() int {
	if r.OutputLimit <= 0 {
		return DefaultOutputLimit
	}
	return r.OutputLimit
}

func Run(ctx context.Context, name string, args ...string) error {
	return RealCommandRunner{}.Run(ctx, name, args...)
}

func RunOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	return RealCommandRunner{}.RunOutput(ctx, name, args...)
}

func LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func TruncateOutput(out []byte) []byte {
	if len(out) <= DefaultOutputLimit {
		return out
	}
	truncated := make([]byte, 0, DefaultOutputLimit+64)
	truncated = append(truncated, out[:DefaultOutputLimit]...)
	truncated = append(truncated, []byte("\n[output truncated]\n")...)
	return truncated
}

func (b *LimitedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		b.limit = DefaultOutputLimit
	}
	accepted := len(p)
	remaining := b.limit - b.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			b.buf.Write(p[:remaining])
			b.truncated = true
		} else {
			b.buf.Write(p)
		}
	} else {
		b.truncated = true
	}
	return accepted, nil
}

func (b *LimitedBuffer) Bytes() []byte {
	out := b.buf.Bytes()
	if !b.truncated {
		return out
	}
	truncated := make([]byte, 0, len(out)+64)
	truncated = append(truncated, out...)
	truncated = append(truncated, []byte("\n[output truncated]\n")...)
	return truncated
}
