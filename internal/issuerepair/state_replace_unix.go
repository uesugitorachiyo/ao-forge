//go:build !windows

package issuerepair

import (
	"os"
	"path/filepath"
)

func replaceStateFile(temporaryPath, statePath string) error {
	if err := os.Rename(temporaryPath, statePath); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(statePath))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
