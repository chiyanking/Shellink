//go:build windows

package sessions

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NewProcessTransport starts a child process and returns a Transport for it.
//
// By default Shellink uses plain anonymous pipes on Windows. ConPTY is only
// used when SHELLINK_CONPTY=1 is set, because on some Windows hosts (including
// machines where ConPTY children never actually start) the pseudo console
// produces no output at all, leaving sessions stuck in CONNECTING forever.
func NewProcessTransport(program string, args []string, env map[string]string, term string, cols, rows int) (Transport, error) {
	if strings.EqualFold(os.Getenv("SHELLINK_CONPTY"), "1") {
		return newConPTYTransport(program, args, env, term, cols, rows)
	}
	return newPipeTransport(program, args, env, term, cols, rows)
}

// ---------------------------------------------------------------------------
// Pipe based transport (default)
// ---------------------------------------------------------------------------

type pipeTransport struct {
	process windows.Handle
	input   windows.Handle
	out     chan string
	done    chan error
	readers sync.WaitGroup
	mu      sync.Mutex
	closed  sync.Once
}

func newPipeTransport(program string, args []string, env map[string]string, term string, cols, rows int) (Transport, error) {
	if cols < 1 {
		cols = 80
	}
	if rows < 1 {
		rows = 24
	}
	var inRead, inWrite, outRead, outWrite, errRead, errWrite windows.Handle
	if err := windows.CreatePipe(&inRead, &inWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}
	if err := windows.CreatePipe(&outRead, &outWrite, nil, 0); err != nil {
		closeHandles(inRead, inWrite)
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	if err := windows.CreatePipe(&errRead, &errWrite, nil, 0); err != nil {
		closeHandles(inRead, inWrite, outRead, outWrite)
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}
	// Only these three handles are inherited by the child.
	childHandles := []windows.Handle{inRead, outWrite, errWrite}
	for _, h := range childHandles {
		if err := windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
			closeHandles(inRead, inWrite, outRead, outWrite, errRead, errWrite)
			return nil, fmt.Errorf("mark handle inheritable: %w", err)
		}
	}
	attributeList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		closeHandles(inRead, inWrite, outRead, outWrite, errRead, errWrite)
		return nil, err
	}
	defer attributeList.Delete()
	if err := attributeList.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST,
		unsafe.Pointer(&childHandles[0]),
		uintptr(len(childHandles))*unsafe.Sizeof(childHandles[0])); err != nil {
		closeHandles(inRead, inWrite, outRead, outWrite, errRead, errWrite)
		return nil, fmt.Errorf("configure handle list: %w", err)
	}

	startup := &windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Cb:        uint32(unsafe.Sizeof(windows.StartupInfoEx{})),
			Flags:     windows.STARTF_USESTDHANDLES,
			StdInput:  inRead,
			StdOutput: outWrite,
			StdErr:    errWrite,
		},
		ProcThreadAttributeList: attributeList.List(),
	}
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
	commandLine := windows.ComposeCommandLine(append([]string{program}, args...))
	commandPtr, err := windows.UTF16PtrFromString(commandLine)
	if err != nil {
		closeHandles(inRead, inWrite, outRead, outWrite, errRead, errWrite)
		return nil, err
	}
	programPtr, err := windows.UTF16PtrFromString(program)
	if err != nil {
		closeHandles(inRead, inWrite, outRead, outWrite, errRead, errWrite)
		return nil, err
	}
	home, _ := os.UserHomeDir()
	var dirPtr *uint16
	if home != "" {
		dirPtr, _ = windows.UTF16PtrFromString(home)
	}
	processInfo := new(windows.ProcessInformation)
	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT | windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_NO_WINDOW)
	if err := windows.CreateProcess(programPtr, commandPtr, nil, nil, true, flags, envPtr, dirPtr, &startup.StartupInfo, processInfo); err != nil {
		closeHandles(inRead, inWrite, outRead, outWrite, errRead, errWrite)
		return nil, fmt.Errorf("start process: %w", err)
	}
	_ = windows.CloseHandle(processInfo.Thread)
	// Close the child ends so reads return EOF/broken pipe when the child exits.
	_ = windows.CloseHandle(inRead)
	_ = windows.CloseHandle(outWrite)
	_ = windows.CloseHandle(errWrite)

	transport := &pipeTransport{
		process: processInfo.Process,
		input:   inWrite,
		out:     make(chan string, 128),
		done:    make(chan error, 1),
	}
	for _, handle := range []windows.Handle{outRead, errRead} {
		transport.readers.Add(1)
		go pumpPipe(handle, transport.out, &transport.readers)
	}
	// The output channel MUST be closed once both pipes hit EOF, otherwise
	// Session.Attach blocks on "range transport.Output()" forever and never
	// observes the process exit, leaving the session stuck in CONNECTING even
	// though the child is long gone.
	go func() {
		transport.readers.Wait()
		close(transport.out)
	}()
	go func() {
		_, _ = windows.WaitForSingleObject(processInfo.Process, windows.INFINITE)
		var code uint32
		if waitErr := windows.GetExitCodeProcess(processInfo.Process, &code); waitErr != nil {
			transport.done <- waitErr
		} else if code != 0 {
			transport.done <- processExitError(int(code))
		} else {
			transport.done <- nil
		}
		close(transport.done)
	}()
	return transport, nil
}

func pumpPipe(handle windows.Handle, out chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	buf := make([]byte, 32*1024)
	for {
		var n uint32
		err := windows.ReadFile(handle, buf, &n, nil)
		if n > 0 {
			out <- string(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func (p *pipeTransport) Write(data string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.input == 0 {
		return errors.New("session input is closed")
	}
	var written uint32
	err := windows.WriteFile(p.input, []byte(data), &written, nil)
	if err != nil {
		return err
	}
	if int(written) != len(data) {
		return fmt.Errorf("short write: %d of %d bytes", written, len(data))
	}
	return nil
}

func (p *pipeTransport) Resize(cols, rows int) error {
	// No local pty to resize when running over pipes.
	return nil
}

func (p *pipeTransport) Close() error {
	var first error
	p.closed.Do(func() {
		p.mu.Lock()
		if p.input != 0 {
			// Closing the write end lets the child see EOF on stdin.
			if err := windows.CloseHandle(p.input); err != nil {
				first = err
			}
			p.input = 0
		}
		p.mu.Unlock()
		if err := windows.TerminateProcess(p.process, 1); err != nil && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			first = errors.Join(first, err)
		}
	})
	return first
}

func (p *pipeTransport) Output() <-chan string { return p.out }
func (p *pipeTransport) Done() <-chan error    { return p.done }

// ---------------------------------------------------------------------------
// ConPTY based transport (opt-in via SHELLINK_CONPTY=1)
// ---------------------------------------------------------------------------

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

func newConPTYTransport(program string, args []string, env map[string]string, term string, cols, rows int) (Transport, error) {
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
		closeHandles(inRead, inWrite, outRead, outWrite)
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

func closeHandles(handles ...windows.Handle) {
	for _, handle := range handles {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}
	}
}
