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
	tokenQuery                     = 0x0008
	tokenElevation                 = 20
	invalidHandleValue             = ^uintptr(0)
)

// The browsers a user is likely to have as default; only these can hold an
// elevated instance that refuses a normal-rights request.
var browserImages = []string{"msedge.exe", "chrome.exe", "firefox.exe"}

type processEntry32 struct {
	size            uint32
	cntUsage        uint32
	processID       uint32
	defaultHeapID   uintptr
	moduleID        uint32
	cntThreads      uint32
	parentProcessID uint32
	priClassBase    int32
	dwFlags         uint32
	szExeFile       [syscall.MAX_PATH]uint16
}

var (
	kernel32      = syscall.NewLazyDLL("kernel32.dll")
	advapi32      = syscall.NewLazyDLL("advapi32.dll")
	procOpen      = kernel32.NewProc("OpenProcess")
	procExitCode  = kernel32.NewProc("GetExitCodeProcess")
	procClose     = kernel32.NewProc("CloseHandle")
	procSelf      = kernel32.NewProc("GetCurrentProcess")
	procOpenTok   = advapi32.NewProc("OpenProcessToken")
	procTokInfo   = advapi32.NewProc("GetTokenInformation")
	procSnapshot  = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcFirst = kernel32.NewProc("Process32FirstW")
	procProcNext  = kernel32.NewProc("Process32NextW")
)

const (
	integrityMedium     = 0x2000
	tokenIntegrityLevel = 25
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
	snap, _, _ := procSnapshot.Call(0x00000002, 0) // TH32CS_SNAPPROCESS
	if snap == invalidHandleValue || snap == 0 {
		return nil
	}
	defer procClose.Call(snap)

	var entry processEntry32
	entry.size = uint32(unsafe.Sizeof(entry))
	ret, _, _ := procProcFirst.Call(snap, uintptr(unsafe.Pointer(&entry)))
	var pids []int
	for ret != 0 {
		name := syscall.UTF16ToString(entry.szExeFile[:])
		if strings.EqualFold(name, image) {
			if pid := int(entry.processID); pid > 0 && alive(pid) {
				pids = append(pids, pid)
			}
		}
		entry.size = uint32(unsafe.Sizeof(entry))
		ret, _, _ = procProcNext.Call(snap, uintptr(unsafe.Pointer(&entry)))
	}
	return pids
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
	for _, image := range browserImages {
		for _, pid := range pidsByName(image) {
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

// integrityRID returns this process's integrity level (0x1000 low, 0x2000
// medium, 0x3000 high), or 0 when it cannot be read.
//
// It exists because Windows derives a process's integrity from the executable:
// a file that carries a low mandatory label - which is what an archive
// extracted into a folder labelled Low inherits - starts at low integrity even
// for an administrator. Such a process can neither see the desktop shell nor
// hand it a URL, so the browser silently never opens while everything else
// (the daemon, the page itself) works fine.
func integrityRID() uint32 {
	self, _, _ := procSelf.Call()
	var token uintptr
	if ret, _, _ := procOpenTok.Call(self, tokenQuery, uintptr(unsafe.Pointer(&token))); ret == 0 {
		return 0
	}
	defer procClose.Call(token)

	var buf [64]byte
	var size uint32
	if ret, _, _ := procTokInfo.Call(token, tokenIntegrityLevel, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&size))); ret == 0 {
		return 0
	}
	// TOKEN_MANDATORY_LABEL starts with a SID_AND_ATTRIBUTES whose Sid field
	// points at a SID inside the same buffer: BYTE revision, BYTE sub authority
	// count, 6 byte identifier authority, then the sub authorities. Integrity
	// SIDs look like S-1-16-<level>, so the last sub authority is the level.
	sid := *(**byte)(unsafe.Pointer(&buf[0]))
	if sid == nil {
		return 0
	}
	count := *(*byte)(unsafe.Add(unsafe.Pointer(sid), 1))
	if count == 0 {
		return 0
	}
	rid := unsafe.Add(unsafe.Pointer(sid), 8+uintptr(count-1)*4)
	return *(*uint32)(rid)
}
