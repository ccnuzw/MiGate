package corefile

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// PathKind is the expected filesystem object type for a path.
type PathKind string

const (
	KindFile PathKind = "file"
	KindDir  PathKind = "directory"
)

// Requirement describes the checks that must pass for a path.
type Requirement struct {
	Kind       PathKind
	Readable   bool
	Writable   bool
	Executable bool
}

// Status is a structured filesystem check result suitable for diagnostics.
type Status struct {
	Path       string
	Exists     bool
	IsFile     bool
	IsDir      bool
	Readable   bool
	Writable   bool
	Executable bool
	Code       string
	Err        error
}

// OK reports whether the path exists, has the requested type, and satisfies
// all requested access checks.
func (s Status) OK() bool {
	return s.Code == ""
}

func (s Status) Error() string {
	if s.OK() {
		return ""
	}
	msg := fmt.Sprintf("%s: %s", s.Code, s.Path)
	if s.Err != nil {
		msg += ": " + s.Err.Error()
	}
	return msg
}

// CheckPath validates path type and access bits without mutating the target.
func CheckPath(path string, req Requirement) Status {
	status := Status{Path: filepath.Clean(path)}
	info, err := os.Stat(status.Path)
	if err != nil {
		status.Err = err
		if os.IsNotExist(err) {
			status.Code = "not_exists"
		} else if os.IsPermission(err) {
			status.Code = "stat_permission_denied"
		} else {
			status.Code = "stat_failed"
		}
		return status
	}
	status.Exists = true
	status.IsDir = info.IsDir()
	status.IsFile = info.Mode().IsRegular()

	switch req.Kind {
	case KindFile:
		if !status.IsFile {
			if status.IsDir {
				status.Code = "not_file"
			} else {
				status.Code = "not_regular_file"
			}
			return status
		}
	case KindDir:
		if !status.IsDir {
			status.Code = "not_directory"
			return status
		}
	}

	if req.Readable {
		if err := unix.Access(status.Path, unix.R_OK); err != nil {
			status.Err = err
			status.Code = "read_permission_denied"
			return status
		}
		status.Readable = true
	}
	if req.Writable {
		if err := unix.Access(status.Path, unix.W_OK); err != nil {
			status.Err = err
			status.Code = "write_permission_denied"
			return status
		}
		status.Writable = true
	}
	if req.Executable {
		if err := unix.Access(status.Path, unix.X_OK); err != nil {
			status.Err = err
			status.Code = "execute_permission_denied"
			return status
		}
		status.Executable = true
	}
	return status
}

// EnsureDir creates a directory if missing, then validates its type and access.
func EnsureDir(path string, req Requirement) Status {
	if err := os.MkdirAll(path, 0755); err != nil {
		status := Status{Path: filepath.Clean(path), Err: err}
		if os.IsPermission(err) {
			status.Code = "create_directory_permission_denied"
		} else {
			status.Code = "create_directory_failed"
		}
		return status
	}
	req.Kind = KindDir
	return CheckPath(path, req)
}
