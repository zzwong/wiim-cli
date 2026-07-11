//go:build windows

package wiim

// preserveExistingFileOwnership is a no-op where ownership preservation is
// unsupported; replacement ownership follows the platform's native semantics.
func preserveExistingFileOwnership(targetPath, replacementPath string) error {
	return nil
}
