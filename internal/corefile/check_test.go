package corefile

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckPathDistinguishesMissingFileDirectoryAndExecutable(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	if got := CheckPath(missing, Requirement{Kind: KindFile, Readable: true}); got.OK() || got.Code != "not_exists" || !strings.Contains(got.Error(), missing) {
		t.Fatalf("missing file check = %+v, error=%q", got, got.Error())
	}

	if got := CheckPath(dir, Requirement{Kind: KindFile}); got.OK() || got.Code != "not_file" {
		t.Fatalf("directory as file check = %+v", got)
	}

	file := filepath.Join(dir, "config.json")
	if err := os.WriteFile(file, []byte("{}"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if got := CheckPath(file, Requirement{Kind: KindDir}); got.OK() || got.Code != "not_directory" {
		t.Fatalf("file as directory check = %+v", got)
	}
	if got := CheckPath(file, Requirement{Kind: KindFile, Readable: true}); !got.OK() || !got.Readable {
		t.Fatalf("readable file check = %+v", got)
	}
	if got := CheckPath(file, Requirement{Kind: KindFile, Executable: true}); got.OK() || got.Code != "execute_permission_denied" {
		t.Fatalf("non-executable file check = %+v", got)
	}
}

func TestCheckPathReportsPermissionProblems(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission-bit checks are not reliable for this test environment")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "config.json")
	if err := os.WriteFile(file, []byte("{}"), 0000); err != nil {
		t.Fatalf("write file: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(file, 0644) })
	if got := CheckPath(file, Requirement{Kind: KindFile, Readable: true}); got.OK() || got.Code != "read_permission_denied" {
		t.Fatalf("unreadable file check = %+v", got)
	}
}

func TestEnsureDirCreatesAndValidatesWritableDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config")
	got := EnsureDir(path, Requirement{Writable: true})
	if !got.OK() || !got.IsDir || !got.Writable {
		t.Fatalf("ensure dir = %+v", got)
	}
}
