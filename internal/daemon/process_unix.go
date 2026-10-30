//go:build !windows

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"shellink/internal/config"
)

func startDetachedPlatform(cfg config.Config) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	logFile, err := prepareLogWithFallback(cfg.LogPath)
	if err != nil {
		return err
	}
	command := exec.Command(executable, "server", "run")
	command.Stdin = nil
	command.Stdout = logFile
	command.Stderr = logFile
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		logFile.Close()
		return err
	}
	_ = logFile.Close()
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release daemon process: %w", err)
	}
	return nil
}
