//go:build !linux && !darwin && !windows

package wiim

// preserveExistingFileOwnership is a no-op on platforms without a supported
// syscall.Stat_t ownership implementation.
func preserveExistingFileOwnership(targetPath, replacementPath string) error {
	return nil
}
