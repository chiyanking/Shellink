//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// Opening the web console is the part of the launcher that depends on
// privileges, and it fails silently when it is done naively:
//
//   - a URL handed from an elevated process to a browser running with normal
//     rights is dropped (Edge even answers "现有实例正在以提升的权限运行" and
//     offers to restart itself, which closes the window the user had);
//   - a browser started *by* an elevated process keeps that elevation for the
//     rest of the session, so every later launch inherits the same problem;
//   - explorer.exe / rundll32 only report that the request was queued, not
//     that a window appeared, so a dropped request still looks like success.
//
// The reliable way out is to create the browser process with the token of the
// desktop shell (explorer.exe): same user, same session, normal rights. That is
// what happens when the user clicks a link themselves, and it works no matter
// how the launcher was started. The remaining methods are fallbacks for the
// rare machine where the shell token cannot be borrowed (SeImpersonatePrivilege
// removed by policy).
var (
	procCreateProcessWithTokenW = advapi32.NewProc("CreateProcessWithTokenW")
	procDuplicateTokenEx        = advapi32.NewProc("DuplicateTokenEx")
	procWaitForSingleObject     = kernel32.NewProc("WaitForSingleObject")
)

const (
	tokenDuplicate     = 0x0002
	tokenAssignPrimary = 0x0001
	tokenAllAccess     = 0x000F01FF
	secImpersonation   = 2
	tokenTypePrimary   = 1
)

type startupInfoW struct {
	cb              uint32
	lpReserved      *uint16
	lpDesktop       *uint16
	lpTitle         *uint16
	dwX             uint32
	dwY             uint32
	dwXSize         uint32
	dwYSize         uint32
	dwXCountChars   uint32
	dwYCountChars   uint32
	dwFillAttribute uint32
	dwFlags         uint32
	wShowWindow     uint16
	cbReserved2     uint16
	lpReserved2     *byte
	hStdInput       syscall.Handle
	hStdOutput      syscall.Handle
	hStdError       syscall.Handle
}

type processInformation struct {
	hProcess    syscall.Handle
	hThread     syscall.Handle
	dwProcessID uint32
	dwThreadID  uint32
}

// openBrowser shows the web console and reports which method was used.
//
// It returns an error only when every method failed, and that error names the
// last reason, so the caller can print the address instead of claiming a page
// was opened.
func openBrowser(url string) (string, error) {
	var firstErr error
	debug := os.Getenv("SHELLINK_BROWSER_DEBUG") != ""
	trace := func(format string, args ...any) {
		if debug {
			fmt.Fprintf(os.Stderr, "[browser] "+format+"\n", args...)
		}
	}
	fail := func(method string, err error) {
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("%s: %w", method, err)
		}
	}
	trace("elevated=%v elevatedBrowser=%v shell=%d", isElevated(), elevatedBrowser(), shellPID())

	// SHELLINK_BROWSER_FORCE pins one method, so a machine where the browser
	// does not show up can be diagnosed without guessing which path ran.
	if forced := strings.ToLower(os.Getenv("SHELLINK_BROWSER_FORCE")); forced != "" {
		trace("forced method: %s", forced)
		switch forced {
		case "shell-token":
			return "shell-token", launchAsShell(url)
		case "rundll32":
			return "rundll32", startDetached(rundll32Path(), "url.dll,FileProtocolHandler", url)
		case "explorer":
			return "explorer", exec.Command("explorer.exe", url).Start()
		case "cmd":
			return "cmd", startDetached("cmd.exe", "/c", "start", "", url)
		}
	}

	if isElevated() {
		// Borrow the shell token first: the browser then runs exactly as it
		// would if the user had clicked the link, and the page is accepted by
		// whatever browser is already open.
		if err := launchAsShell(url); err == nil {
			return "shell-token", nil
		} else {
			trace("shell-token failed: %v", err)
			fail("shell-token", err)
		}
		if err := exec.Command("explorer.exe", url).Start(); err == nil {
			return "explorer", nil
		} else {
			trace("explorer failed: %v", err)
			fail("explorer", err)
		}
	}

	if err := startDetached(rundll32Path(), "url.dll,FileProtocolHandler", url); err == nil {
		return "rundll32", nil
	} else {
		trace("rundll32 failed: %v", err)
		fail("rundll32", err)
	}

	if err := exec.Command("explorer.exe", url).Start(); err == nil {
		return "explorer", nil
	} else {
		trace("explorer failed: %v", err)
		fail("explorer", err)
	}
	return "", firstErr
}

// launchAsShell starts a URL handler with the token of the desktop shell, so
// the request reaches the browser with the rights it expects.
func launchAsShell(url string) error {
	pid := shellPID()
	if pid == 0 {
		return errors.New("没有找到资源管理器进程")
	}
	handle, _, _ := procOpen.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if handle == 0 {
		return errors.New("无法打开资源管理器进程")
	}
	defer procClose.Call(handle)

	var token uintptr
	ret, _, errno := procOpenTok.Call(handle, tokenQuery|tokenDuplicate|tokenAssignPrimary, uintptr(unsafe.Pointer(&token)))
	if ret == 0 {
		return fmt.Errorf("无法读取资源管理器令牌: %v", errno)
	}
	defer procClose.Call(token)

	var dup uintptr
	ret, _, errno = procDuplicateTokenEx.Call(token, tokenAllAccess, 0, secImpersonation, tokenTypePrimary, uintptr(unsafe.Pointer(&dup)))
	if ret == 0 {
		return fmt.Errorf("无法复制资源管理器令牌: %v", errno)
	}
	defer procClose.Call(dup)

	app := rundll32Path()
	cmdLine := fmt.Sprintf(`"%s" url.dll,FileProtocolHandler %s`, app, url)
	appPtr, _ := syscall.UTF16PtrFromString(app)
	cmdPtr, _ := syscall.UTF16PtrFromString(cmdLine)
	deskPtr, _ := syscall.UTF16PtrFromString(`winsta0\default`)

	si := startupInfoW{cb: uint32(unsafe.Sizeof(startupInfoW{})), lpDesktop: deskPtr}
	var pi processInformation
	ret, _, errno = procCreateProcessWithTokenW.Call(
		dup, 0,
		uintptr(unsafe.Pointer(appPtr)),
		uintptr(unsafe.Pointer(cmdPtr)),
		0, 0, 0,
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	)
	if ret == 0 {
		return fmt.Errorf("以普通权限启动浏览器失败: %v", errno)
	}
	if pi.hThread != 0 {
		procClose.Call(uintptr(pi.hThread))
	}
	if pi.hProcess != 0 {
		procClose.Call(uintptr(pi.hProcess))
	}
	return nil
}

// shellPID returns the pid of the desktop shell, or 0 when it is not running.
func shellPID() int {
	pids := pidsByName("explorer.exe")
	if len(pids) == 0 {
		return 0
	}
	return pids[0]
}

func rundll32Path() string {
	if root := os.Getenv("SystemRoot"); root != "" {
		p := filepath.Join(root, "System32", "rundll32.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "rundll32.exe"
}

// startDetached runs a helper and does not wait for it, but still surfaces the
// "cannot start at all" case instead of reporting a success that never
// happened.
func startDetached(name string, args ...string) error {
	c := exec.Command(name, args...)
	if err := c.Start(); err != nil {
		return err
	}
	go func() { _ = c.Wait() }()
	return nil
}
