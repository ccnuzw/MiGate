package panelconfig

import (
	"os"
	"os/user"
	"strconv"
)

const FileMode os.FileMode = 0o640

func WriteFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, FileMode); err != nil {
		return err
	}
	if os.Geteuid() == 0 {
		if group, err := user.LookupGroup("migate"); err == nil {
			if gid, err := strconv.Atoi(group.Gid); err == nil {
				_ = os.Chown(path, 0, gid)
			}
		}
	}
	return os.Chmod(path, FileMode)
}
