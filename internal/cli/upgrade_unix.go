//go:build !windows

package cli

import (
	"fmt"
	"os"

	"shellink/internal/config"
	"shellink/internal/daemon"
)

func replaceExecutable(tempPath, targetPath string, cfg config.Config, restart bool) error {
	info, err := os.Stat(targetPath)
	if err != nil {
		return err
	}
	if err := os.Chmod(tempPath, info.Mode().Perm()|0100); err != nil {
		return err
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		return fmt.Errorf("replace executable: %w", err)
	}
	if restart {
		connection, err := daemon.Ensure(cfg, 15_000_000_000)
		if err != nil {
			return fmt.Errorf("updated binary, but could not restart the daemon: %w", err)
		}
		connection.Close()
	}
	return nil
}
