//go:build windows

package wiim

// preserveExistingFileOwnership is a no-op on Windows, where os.Chown is not
// supported and replacement ownership follows Windows ACL semantics.
func preserveExistingFileOwnership(targetPath, replacementPath string) error {
	return nil
}
