//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// procDiskFree is GetDiskFreeSpaceExW; the syscall package does not wrap it.
var procDiskFree = kernel32.NewProc("GetDiskFreeSpaceExW")

// osVersion returns a readable Windows version and its build number.
//
// The build number is what decides ConPTY support; "ver" is the least invasive
// way to read it, and its numeric part is identical in every locale (only the
// surrounding words are translated).
func osVersion() (string, int) {
	out, err := exec.Command("cmd", "/c", "ver").Output()
	if err != nil {
		return "Windows", 0
	}
	match := regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`).FindStringSubmatch(string(out))
	if match == nil {
		return "Windows", 0
	}
	build, err := strconv.Atoi(match[3])
	if err != nil {
		return "Windows", 0
	}
	return fmt.Sprintf("Windows %s.%s.%s", match[1], match[2], match[3]), build
}

// freeDiskBytes reports the space available on the volume holding path.
//
// The syscall package has no wrapper for this call, so it goes through
// kernel32 directly; the quota-aware "available to caller" value is the one
// that matters, since a quota can be far smaller than the free space.
func freeDiskBytes(path string) (uint64, bool) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return 0, false
	}
	ptr, err := syscall.UTF16PtrFromString(absolute)
	if err != nil {
		return 0, false
	}
	var available, total, free uint64
	result, _, _ := procDiskFree.Call(
		uintptr(unsafe.Pointer(ptr)),
		uintptr(unsafe.Pointer(&available)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&free)),
	)
	if result == 0 {
		return 0, false
	}
	return available, true
}

// findSSHClient locates the OpenSSH client and reads its version.
//
// It is the one dependency Shellink does not carry itself: SSH profiles run the
// system client, so a machine without the "OpenSSH Client" optional feature can
// still open command sessions but cannot reach any server.
func findSSHClient() (string, string) {
	var candidates []string
	for _, name := range []string{"ssh", "ssh.exe"} {
		if path, err := exec.LookPath(name); err == nil {
			candidates = append(candidates, path)
		}
	}
	// Windows 10+ installs the client here but only adds it to PATH for new
	// shells, so a launcher started from an older session would miss it.
	if root := os.Getenv("SystemRoot"); root != "" {
		candidates = append(candidates, filepath.Join(root, "System32", "OpenSSH", "ssh.exe"))
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			continue
		}
		return path, sshVersion(path)
	}
	return "", ""
}

// sshVersion returns the first line of "ssh -V", which the client prints on
// stderr: "OpenSSH_for_Windows_9.5p1, LibreSSL 3.8.2".
func sshVersion(path string) string {
	out, err := exec.Command(path, "-V").CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
}
