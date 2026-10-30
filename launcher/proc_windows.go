//go:build windows

package main

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const (
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
	daemonImage                    = "shellink.exe"
)

var (
	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	procOpen     = kernel32.NewProc("OpenProcess")
	procExitCode = kernel32.NewProc("GetExitCodeProcess")
	procClose    = kernel32.NewProc("CloseHandle")
)

// runningShellink lists every live process started from shellink.exe.
//
// Matching by image name (instead of the data directory) is deliberate: a copy
// of the folder somewhere else is still the same single-instance daemon
// fighting for the same port, and that leftover is exactly what makes a start
// fail.
func runningShellink() []int {
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq "+daemonImage, "/FO", "CSV", "/NH").Output()
	if err != nil {
		return nil
	}
	var pids []int
	seen := map[int]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := csvFields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(fields[1]))
		if err != nil || pid <= 0 || seen[pid] {
			continue
		}
		if alive(pid) {
			seen[pid] = true
			pids = append(pids, pid)
		}
	}
	return pids
}

// csvFields splits one tasklist /FO CSV line. Fields are quoted, and the memory
// column contains a comma ("43,860 K"), so splitting on `","` is required.
func csvFields(line string) []string {
	line = strings.TrimSpace(line)
	if line == "" || !strings.HasPrefix(line, "\"") {
		return nil
	}
	return strings.Split(strings.Trim(line, "\""), "\",\"")
}

func alive(pid int) bool {
	handle, _, _ := procOpen.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if handle == 0 {
		return false
	}
	defer procClose.Call(handle)
	var code uint32
	ret, _, _ := procExitCode.Call(handle, uintptr(unsafe.Pointer(&code)))
	if ret == 0 {
		return false
	}
	return code == stillActive
}

func forceKill(pid int) {
	_ = exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
}
