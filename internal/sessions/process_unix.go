//go:build !windows

package sessions

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func NewProcessTransport(program string, args []string, env map[string]string, term string, cols, rows int) (Transport, error) {
	command, err := platformCommand(program, args, env, term, cols, rows)
	if err != nil {
		return nil, err
	}
	return newPipeTransport(command)
}

func platformCommand(program string, args []string, env map[string]string, term string, cols, rows int) (*exec.Cmd, error) {
	commandLine := quoteShellArg(program)
	for _, arg := range args {
		commandLine += " " + quoteShellArg(arg)
	}
	var command *exec.Cmd
	if script, err := exec.LookPath("script"); err == nil {
		if runtime.GOOS == "darwin" {
			command = exec.Command(script, "-q", "/dev/null", "/bin/sh", "-lc", commandLine)
		} else {
			command = exec.Command(script, "-q", "-c", commandLine, "/dev/null")
		}
	} else {
		command = exec.Command(program, args...)
	}
	processEnv := map[string]string{"TERM": term, "COLUMNS": fmt.Sprint(max(cols, 2000)), "LINES": fmt.Sprint(rows)}
	for key, value := range env {
		processEnv[key] = value
	}
	command.Env = environment(processEnv)
	command.Dir, _ = os.UserHomeDir()
	return command, nil
}

func resizePlatform(_ *exec.Cmd, _ int, _ int) error { return nil }

func quoteShellArg(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
