//go:build windows

package sessions

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

type conptyTransport struct {
	process windows.Handle
	console windows.Handle
	input   *os.File
	output  *os.File
	out     chan string
	done    chan error
	mu      sync.Mutex
	closed  sync.Once
}

func NewProcessTransport(program string, args []string, env map[string]string, term string, cols, rows int) (Transport, error) {
	if cols < 1 {
		cols = 80
	}
	if rows < 1 {
		rows = 24
	}
	var inRead, inWrite, outRead, outWrite windows.Handle
	if err := windows.CreatePipe(&inRead, &inWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("create ConPTY input pipe: %w", err)
	}
	cleanup := func() {
		for _, handle := range []windows.Handle{inRead, inWrite, outRead, outWrite} {
			if handle != 0 {
				_ = windows.CloseHandle(handle)
			}
		}
	}
	if err := windows.CreatePipe(&outRead, &outWrite, nil, 0); err != nil {
		cleanup()
		return nil, fmt.Errorf("create ConPTY output pipe: %w", err)
	}
	var console windows.Handle
	if err := windows.CreatePseudoConsole(windows.Coord{X: int16(cols), Y: int16(rows)}, inRead, outWrite, 0, &console); err != nil {
		cleanup()
		return nil, fmt.Errorf("create Windows pseudo console (ConPTY requires Windows 10 1809 or newer): %w", err)
	}
	_ = windows.CloseHandle(inRead)
	_ = windows.CloseHandle(outWrite)
	inRead, outWrite = 0, 0
	attributeList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		windows.ClosePseudoConsole(console)
		cleanup()
		return nil, err
	}
	defer attributeList.Delete()
	if err := attributeList.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, unsafe.Pointer(&console), unsafe.Sizeof(console)); err != nil {
		windows.ClosePseudoConsole(console)
		cleanup()
		return nil, fmt.Errorf("configure ConPTY process: %w", err)
	}
	commandLine := windows.ComposeCommandLine(append([]string{program}, args...))
	commandPtr, err := windows.UTF16PtrFromString(commandLine)
	if err != nil {
		windows.ClosePseudoConsole(console)
		cleanup()
		return nil, err
	}
	programPtr, err := windows.UTF16PtrFromString(program)
	if err != nil {
		windows.ClosePseudoConsole(console)
		cleanup()
		return nil, err
	}
	startup := &windows.StartupInfoEx{
		StartupInfo:             windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{}))},
		ProcThreadAttributeList: attributeList.List(),
	}
	processInfo := new(windows.ProcessInformation)
	overrides := map[string]string{"TERM": term, "COLUMNS": fmt.Sprint(cols), "LINES": fmt.Sprint(rows)}
	for key, value := range env {
		overrides[key] = value
	}
	envValues := environment(overrides)
	sort.Strings(envValues)
	envBlock := utf16.Encode([]rune(strings.Join(envValues, "\x00") + "\x00\x00"))
	var envPtr *uint16
	if len(envBlock) > 0 {
		envPtr = &envBlock[0]
	}
	home, _ := os.UserHomeDir()
	var dirPtr *uint16
	if home != "" {
		dirPtr, _ = windows.UTF16PtrFromString(home)
	}
	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT | windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_NEW_PROCESS_GROUP)
	if err := windows.CreateProcess(programPtr, commandPtr, nil, nil, false, flags, envPtr, dirPtr, &startup.StartupInfo, processInfo); err != nil {
		windows.ClosePseudoConsole(console)
		cleanup()
		return nil, fmt.Errorf("start process in ConPTY: %w", err)
	}
	_ = windows.CloseHandle(processInfo.Thread)
	_ = windows.CloseHandle(outWrite)
	_ = windows.CloseHandle(inRead)
	input := os.NewFile(uintptr(inWrite), "shellink-conpty-input")
	output := os.NewFile(uintptr(outRead), "shellink-conpty-output")
	transport := &conptyTransport{process: processInfo.Process, console: console, input: input, output: output, out: make(chan string, 128), done: make(chan error, 1)}
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, readErr := output.Read(buf)
			if n > 0 {
				transport.out <- string(buf[:n])
			}
			if readErr != nil {
				break
			}
		}
		_ = output.Close()
		close(transport.out)
	}()
	go func() {
		_, _ = windows.WaitForSingleObject(processInfo.Process, windows.INFINITE)
		var code uint32
		waitErr := windows.GetExitCodeProcess(processInfo.Process, &code)
		if waitErr != nil {
			transport.done <- waitErr
		} else if code != 0 {
			transport.done <- processExitError(int(code))
		} else {
			transport.done <- nil
		}
		close(transport.done)
		_ = windows.CloseHandle(processInfo.Process)
	}()
	return transport, nil
}

func (c *conptyTransport) Write(data string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.input == nil {
		return errors.New("session input is closed")
	}
	_, err := c.input.Write([]byte(data))
	return err
}
func (c *conptyTransport) Resize(cols, rows int) error {
	if cols < 1 || rows < 1 {
		return nil
	}
	return windows.ResizePseudoConsole(c.console, windows.Coord{X: int16(cols), Y: int16(rows)})
}
func (c *conptyTransport) Close() error {
	var first error
	c.closed.Do(func() {
		c.mu.Lock()
		if c.input != nil {
			first = c.input.Close()
			c.input = nil
		}
		c.mu.Unlock()
		if err := windows.TerminateProcess(c.process, 1); err != nil && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			first = errors.Join(first, err)
		}
		windows.ClosePseudoConsole(c.console)
	})
	return first
}
func (c *conptyTransport) Output() <-chan string { return c.out }
func (c *conptyTransport) Done() <-chan error    { return c.done }

type processExitError int

func (e processExitError) Error() string { return fmt.Sprintf("process exited with code %d", int(e)) }
func (e processExitError) ExitCode() int { return int(e) }

func platformCommand(program string, args []string, env map[string]string, term string, cols, rows int) (*exec.Cmd, error) {
	command := exec.Command(program, args...)
	processEnv := map[string]string{"TERM": term, "COLUMNS": fmt.Sprint(cols), "LINES": fmt.Sprint(rows)}
	for key, value := range env {
		processEnv[key] = value
	}
	command.Env = environment(processEnv)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	command.Dir, _ = os.UserHomeDir()
	return command, nil
}

func resizePlatform(_ *exec.Cmd, _ int, _ int) error { return nil }
