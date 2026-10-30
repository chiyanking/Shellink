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
	browserImage                   = "msedge.exe"
	tokenQuery                     = 0x0008
	tokenElevation                 = 20
)

var (
	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	advapi32     = syscall.NewLazyDLL("advapi32.dll")
	procOpen     = kernel32.NewProc("OpenProcess")
	procExitCode = kernel32.NewProc("GetExitCodeProcess")
	procClose    = kernel32.NewProc("CloseHandle")
	procSelf     = kernel32.NewProc("GetCurrentProcess")
	procOpenTok  = advapi32.NewProc("OpenProcessToken")
	procTokInfo  = advapi32.NewProc("GetTokenInformation")
)

// runningShellink lists every live process started from shellink.exe.
//
// Matching by image name (instead of the data directory) is deliberate: a copy
// of the folder somewhere else is still the same single-instance daemon
// fighting for the same port, and that leftover is exactly what makes a start
// fail.
func runningShellink() []int {
	return pidsByName(daemonImage)
}

func pidsByName(image string) []int {
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq "+image, "/FO", "CSV", "/NH").Output()
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

// isElevated reports whether the launcher itself runs with an administrator
// token.
//
// This decides how the browser is opened: Windows will not hand a URL from an
// elevated process to a browser running with normal rights, and a browser
// started from an elevated process keeps that elevation for the whole session,
// which then breaks every later "just open the page" for the user.
func isElevated() bool {
	self, _, _ := procSelf.Call()
	return handleElevated(uintptr(self))
}

// elevatedBrowser reports whether any browser process is currently running with
// administrator rights.
//
// Such an instance refuses URLs sent from a normal process, so Edge answers
// with "现有实例正在以提升的权限运行" and the page never opens. It is a
// leftover of an earlier elevated launch, and only the user can close it.
func elevatedBrowser() bool {
	for _, pid := range pidsByName(browserImage) {
		handle, _, _ := procOpen.Call(processQueryLimitedInformation, 0, uintptr(pid))
		if handle == 0 {
			continue
		}
		elevated := handleElevated(handle)
		procClose.Call(handle)
		if elevated {
			return true
		}
	}
	return false
}

func handleElevated(handle uintptr) bool {
	var token uintptr
	if ret, _, _ := procOpenTok.Call(handle, tokenQuery, uintptr(unsafe.Pointer(&token))); ret == 0 {
		return false
	}
	defer procClose.Call(token)
	var info struct{ elevated uint32 }
	var size uint32
	ret, _, _ := procTokInfo.Call(token, tokenElevation, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), uintptr(unsafe.Pointer(&size)))
	if ret == 0 {
		return false
	}
	return info.elevated != 0
}
