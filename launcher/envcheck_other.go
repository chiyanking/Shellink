//go:build !windows

package main

import (
	"os/exec"
	"runtime"
	"strings"
)

func osVersion() (string, int) {
	return runtime.GOOS, 0
}

func freeDiskBytes(string) (uint64, bool) {
	return 0, false
}

func findSSHClient() (string, string) {
	path, err := exec.LookPath("ssh")
	if err != nil {
		return "", ""
	}
	out, err := exec.Command(path, "-V").CombinedOutput()
	if err != nil && len(out) == 0 {
		return path, ""
	}
	return path, strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
}
