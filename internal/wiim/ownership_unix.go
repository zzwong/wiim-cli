//go:build linux || darwin

package wiim

import (
	"errors"
	"os"
	"syscall"
)

// preserveExistingFileOwnership applies the target's owner and group to the
// replacement file before it is atomically renamed into place.
func preserveExistingFileOwnership(targetPath, replacementPath string) error {
	info, err := os.Stat(targetPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	return os.Chown(replacementPath, int(stat.Uid), int(stat.Gid))
}
