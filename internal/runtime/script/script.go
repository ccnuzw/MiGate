package script

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	runtimecmd "github.com/imzyb/MiGate/internal/runtime/command"
)

const DefaultTimeout = 5 * time.Minute

type CommandRunner interface {
	RunInput(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error)
	LookPath(file string) (string, error)
	Stat(name string) (os.FileInfo, error)
	Getpid() int
	Now() time.Time
}

type Runner struct {
	CommandRunner     CommandRunner
	Timeout           time.Duration
	DisableSystemdRun bool
	UnitPrefix        string
}

type RealCommandRunner struct {
	Timeout     time.Duration
	OutputLimit int
}

func (r Runner) RunBash(ctx context.Context, body string) ([]byte, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	commandRunner := r.commandRunner()
	if r.useSystemdRun(commandRunner) {
		unitPrefix := strings.TrimSpace(r.UnitPrefix)
		if unitPrefix == "" {
			unitPrefix = "migate-core"
		}
		unit := fmt.Sprintf("%s-%d-%d", unitPrefix, commandRunner.Getpid(), commandRunner.Now().UnixNano())
		return commandRunner.RunInput(
			ctx,
			strings.NewReader(body),
			"systemd-run",
			"--wait",
			"--pipe",
			"--quiet",
			"--unit="+unit,
			"--collect",
			"--property=Type=oneshot",
			"--property=User=root",
			"--property=TimeoutSec="+systemdTimeoutSec(timeout),
			"bash",
			"-s",
		)
	}
	return commandRunner.RunInput(ctx, strings.NewReader(body), "bash", "-s")
}

func (r Runner) commandRunner() CommandRunner {
	if r.CommandRunner != nil {
		return r.CommandRunner
	}
	return RealCommandRunner{OutputLimit: runtimecmd.DefaultOutputLimit}
}

func (r Runner) useSystemdRun(commandRunner CommandRunner) bool {
	if r.DisableSystemdRun {
		return false
	}
	if _, err := commandRunner.LookPath("systemd-run"); err != nil {
		return false
	}
	if _, err := commandRunner.Stat("/run/systemd/system"); err != nil {
		return false
	}
	return true
}

func systemdTimeoutSec(timeout time.Duration) string {
	seconds := int64(timeout / time.Second)
	if timeout%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		seconds = 1
	}
	return fmt.Sprintf("%d", seconds)
}

func (r RealCommandRunner) RunInput(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = stdin
	out := runtimecmd.NewLimitedBuffer(r.outputLimit())
	cmd.Stdout = out
	cmd.Stderr = out
	err := cmd.Run()
	return out.Bytes(), err
}

func (r RealCommandRunner) LookPath(file string) (string, error) {
	return runtimecmd.LookPath(file)
}

func (r RealCommandRunner) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (r RealCommandRunner) Getpid() int {
	return os.Getpid()
}

func (r RealCommandRunner) Now() time.Time {
	return time.Now()
}

func (r RealCommandRunner) outputLimit() int {
	if r.OutputLimit <= 0 {
		return runtimecmd.DefaultOutputLimit
	}
	return r.OutputLimit
}
