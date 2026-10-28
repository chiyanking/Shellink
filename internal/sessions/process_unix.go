//go:build !windows

package sessions

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

type ptyTransport struct {
	command  *exec.Cmd
	terminal *os.File
	output   chan string
	done     chan error
	mu       sync.Mutex
	closed   bool
	close    sync.Once
}

func NewProcessTransport(program string, args []string, env map[string]string, term string, cols, rows int) (Transport, error) {
	if cols < 1 {
		cols = 80
	}
	if rows < 1 {
		rows = 24
	}
	command := exec.Command(program, args...)
	processEnv := map[string]string{"TERM": term, "COLUMNS": fmt.Sprint(cols), "LINES": fmt.Sprint(rows)}
	for key, value := range env {
		processEnv[key] = value
	}
	command.Env = environment(processEnv)
	command.Dir, _ = os.UserHomeDir()
	// Configure a controlling PTY before starting ssh/the shell, so its initial
	// window size and later SIGWINCH notifications match the attached client.
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, fmt.Errorf("start terminal process: %w", err)
	}
	transport := &ptyTransport{
		command: command, terminal: terminal,
		output: make(chan string, 128), done: make(chan error, 1),
	}
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, readErr := terminal.Read(buf)
			if n > 0 {
				transport.output <- string(buf[:n])
			}
			if readErr != nil {
				break
			}
		}
		// Drain the PTY before publishing the exit status to preserve final output.
		waitErr := command.Wait()
		_ = transport.Close()
		close(transport.output)
		transport.done <- waitErr
		close(transport.done)
	}()
	return transport, nil
}

func (p *ptyTransport) Write(data string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errors.New("session input is closed")
	}
	_, err := io.WriteString(p.terminal, data)
	return err
}

func (p *ptyTransport) Resize(cols, rows int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errors.New("session terminal is closed")
	}
	if cols < 1 || cols > 65535 || rows < 1 || rows > 65535 {
		return errors.New("terminal dimensions are invalid")
	}
	return pty.Setsize(p.terminal, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (p *ptyTransport) Output() <-chan string { return p.output }
func (p *ptyTransport) Done() <-chan error    { return p.done }

func (p *ptyTransport) Close() error {
	var err error
	p.close.Do(func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.closed = true
		if killErr := p.command.Process.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			err = killErr
		}
		err = errors.Join(err, p.terminal.Close())
	})
	return err
}
