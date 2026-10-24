package sessions

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type Transport interface {
	Write(string) error
	Resize(cols, rows int) error
	Close() error
	Output() <-chan string
	Done() <-chan error
}

type processTransport struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	output  chan string
	done    chan error
	writeMu sync.Mutex
	close   sync.Once
}

func newPipeTransport(command *exec.Cmd) (Transport, error) {
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	transport := &processTransport{command: command, stdin: stdin, output: make(chan string, 128), done: make(chan error, 1)}
	var readers sync.WaitGroup
	readers.Add(2)
	read := func(r io.Reader) {
		defer readers.Done()
		buf := make([]byte, 32*1024)
		for {
			n, readErr := r.Read(buf)
			if n > 0 {
				transport.output <- string(buf[:n])
			}
			if readErr != nil {
				return
			}
		}
	}
	go read(stdout)
	go read(stderr)
	go func() {
		waitErr := command.Wait()
		readers.Wait()
		close(transport.output)
		if waitErr != nil {
			var exitErr *exec.ExitError
			if errors.As(waitErr, &exitErr) {
				transport.done <- exitErr
			} else {
				transport.done <- waitErr
			}
		} else {
			transport.done <- nil
		}
		close(transport.done)
	}()
	return transport, nil
}

func (p *processTransport) Write(data string) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if p.stdin == nil {
		return errors.New("session input is closed")
	}
	_, err := io.WriteString(p.stdin, data)
	return err
}

func (p *processTransport) Resize(cols, rows int) error { return resizePlatform(p.command, cols, rows) }
func (p *processTransport) Output() <-chan string       { return p.output }
func (p *processTransport) Done() <-chan error          { return p.done }

func (p *processTransport) Close() error {
	var err error
	p.close.Do(func() {
		p.writeMu.Lock()
		if p.stdin != nil {
			err = p.stdin.Close()
			p.stdin = nil
		}
		p.writeMu.Unlock()
		if p.command.Process != nil {
			if killErr := p.command.Process.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
				err = errors.Join(err, killErr)
			}
		}
	})
	return err
}

func environment(overrides map[string]string) []string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		if index := strings.IndexByte(entry, '='); index > 0 {
			values[entry[:index]] = entry[index+1:]
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}
