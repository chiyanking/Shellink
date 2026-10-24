//go:build windows

package cli

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unicode/utf16"

	"shellink/internal/config"
)

const upgradeDetachedProcess = 0x00000008
const upgradeNewProcessGroup = 0x00000200

func replaceExecutable(tempPath, targetPath string, _ config.Config, restart bool) error {
	if _, err := os.Stat(targetPath); err != nil {
		return err
	}
	powershellQuote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
	script := "Start-Sleep -Seconds 2; Move-Item -Force " + powershellQuote(tempPath) + " " + powershellQuote(targetPath)
	if restart {
		script += "; Start-Process -FilePath " + powershellQuote(targetPath) + " -ArgumentList @('server','start') -WindowStyle Hidden"
	}
	encoded := utf16.Encode([]rune(script))
	bytes := make([]byte, len(encoded)*2)
	for i, value := range encoded {
		bytes[i*2] = byte(value)
		bytes[i*2+1] = byte(value >> 8)
	}
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(bytes))
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: upgradeDetachedProcess | upgradeNewProcessGroup, HideWindow: true}
	command.Stdin, command.Stdout, command.Stderr = nil, nil, nil
	if err := command.Start(); err != nil {
		return fmt.Errorf("schedule executable replacement: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release upgrade helper: %w", err)
	}
	return nil
}
