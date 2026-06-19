package script

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	runtimecmd "github.com/imzyb/MiGate/internal/runtime/command"
)

type fakeCommandRunner struct {
	pathErr error
	statErr error
	out     []byte
	err     error
	calls   []scriptCall
	ctxs    []context.Context
}

type scriptCall struct {
	name  string
	args  []string
	stdin string
}

func (r *fakeCommandRunner) RunInput(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	body, _ := io.ReadAll(stdin)
	r.ctxs = append(r.ctxs, ctx)
	r.calls = append(r.calls, scriptCall{name: name, args: append([]string(nil), args...), stdin: string(body)})
	return r.out, r.err
}

func (r *fakeCommandRunner) LookPath(file string) (string, error) {
	if r.pathErr != nil {
		return "", r.pathErr
	}
	return "/usr/bin/" + file, nil
}

func (r *fakeCommandRunner) Stat(name string) (os.FileInfo, error) {
	if r.statErr != nil {
		return nil, r.statErr
	}
	return fakeFileInfo{mode: os.ModeDir | 0755}, nil
}

func (r *fakeCommandRunner) Getpid() int {
	return 123
}

func (r *fakeCommandRunner) Now() time.Time {
	return time.Unix(456, 789)
}

func TestRunBashUsesSystemdRunPipeWhenAvailable(t *testing.T) {
	fake := &fakeCommandRunner{out: []byte("ok\n")}
	out, err := (Runner{CommandRunner: fake}).RunBash(context.Background(), "echo ok")
	if err != nil {
		t.Fatalf("RunBash returned error: %v", err)
	}
	if string(out) != "ok\n" {
		t.Fatalf("unexpected output: %q", string(out))
	}
	if len(fake.calls) != 1 {
		t.Fatalf("calls = %#v, want one", fake.calls)
	}
	call := fake.calls[0]
	if call.name != "systemd-run" || call.stdin != "echo ok" {
		t.Fatalf("unexpected call: %#v", call)
	}
	args := strings.Join(call.args, "\n")
	for _, want := range []string{
		"--wait",
		"--pipe",
		"--quiet",
		"--unit=migate-core-123-456000000789",
		"--collect",
		"--property=Type=oneshot",
		"--property=User=root",
		"--property=TimeoutSec=300",
		"bash",
		"-s",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("systemd-run args missing %q: %#v", want, call.args)
		}
	}
}

func TestRunBashFallsBackToBashWhenSystemdRunUnavailable(t *testing.T) {
	fake := &fakeCommandRunner{pathErr: errors.New("missing")}
	_, err := (Runner{CommandRunner: fake}).RunBash(context.Background(), "echo ok")
	if err != nil {
		t.Fatalf("RunBash returned error: %v", err)
	}
	if len(fake.calls) != 1 || fake.calls[0].name != "bash" || strings.Join(fake.calls[0].args, " ") != "-s" {
		t.Fatalf("unexpected fallback call: %#v", fake.calls)
	}
}

func TestRunBashPassesCancelableTimeoutContext(t *testing.T) {
	fake := &fakeCommandRunner{}
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := (Runner{CommandRunner: fake, Timeout: time.Minute}).RunBash(parent, "echo ok")
	if err != nil {
		t.Fatalf("RunBash returned error: %v", err)
	}
	if len(fake.ctxs) != 1 {
		t.Fatalf("ctxs = %#v, want one", fake.ctxs)
	}
	if _, ok := fake.ctxs[0].Deadline(); !ok {
		t.Fatal("runner context must have deadline")
	}
	cancel()
	select {
	case <-fake.ctxs[0].Done():
	case <-time.After(time.Second):
		t.Fatal("runner context did not inherit parent cancellation")
	}
}

func TestRunBashUsesConfiguredSystemdTimeout(t *testing.T) {
	fake := &fakeCommandRunner{}
	_, err := (Runner{CommandRunner: fake, Timeout: 90 * time.Second}).RunBash(context.Background(), "echo ok")
	if err != nil {
		t.Fatalf("RunBash returned error: %v", err)
	}
	args := strings.Join(fake.calls[0].args, "\n")
	if !strings.Contains(args, "--property=TimeoutSec=90") {
		t.Fatalf("systemd-run args missing configured timeout: %#v", fake.calls[0].args)
	}
}

func TestLimitedBufferTruncatesOutput(t *testing.T) {
	buf := runtimecmd.NewLimitedBuffer(4)
	n, err := buf.Write([]byte("abcdef"))
	if err != nil || n != 6 {
		t.Fatalf("Write = %d, %v", n, err)
	}
	got := string(buf.Bytes())
	if got != "abcd\n[output truncated]\n" {
		t.Fatalf("unexpected truncated output: %q", got)
	}
}

type fakeFileInfo struct {
	mode os.FileMode
}

func (f fakeFileInfo) Name() string       { return "fake" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeFileInfo) Sys() interface{}   { return nil }
