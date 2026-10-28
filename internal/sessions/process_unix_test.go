//go:build !windows

package sessions

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func waitForProcessOutput(t *testing.T, transport Transport, want string) string {
	t.Helper()
	var output strings.Builder
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case chunk, ok := <-transport.Output():
			if !ok {
				t.Fatalf("process output closed waiting for %q: %q", want, output.String())
			}
			output.WriteString(chunk)
			if strings.Contains(output.String(), want) {
				return output.String()
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %q: %q", want, output.String())
		}
	}
}

func TestProcessTerminalSize(t *testing.T) {
	// Read the kernel's PTY dimensions, as ssh and readline do. Environment
	// variables alone do not configure terminal wrapping or resize propagation.
	transport, err := NewProcessTransport("/bin/sh", []string{"-c", `
stty -echo
trap 'printf "WINCH:"; stty size' WINCH
printf 'SIZE:'; stty size
printf 'ENV:%s:%s:%s:%s\n' "$TERM" "$COLUMNS" "$LINES" "$SHELLINK_TEST_VALUE"
while :; do
  IFS= read -r line || continue
  printf 'SIZE:'; stty size
done
`}, map[string]string{"SHELLINK_TEST_VALUE": "value with 'quotes' and spaces"}, "xterm-256color", 93, 31)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = transport.Close() })
	output := waitForProcessOutput(t, transport, "ENV:xterm-256color:93:31:value with 'quotes' and spaces\r\n")
	if !strings.Contains(output, "SIZE:31 93\r\n") {
		t.Fatalf("initial PTY dimensions were not applied: %q", output)
	}
	for _, size := range []struct {
		cols, rows int
		want       string
	}{
		{132, 42, "SIZE:42 132\r\n"},
		{40, 12, "SIZE:12 40\r\n"},
	} {
		if err := transport.Resize(size.cols, size.rows); err != nil {
			t.Fatal(err)
		}
		waitForProcessOutput(t, transport, strings.Replace(size.want, "SIZE:", "WINCH:", 1))
		if err := transport.Write("size\n"); err != nil {
			t.Fatal(err)
		}
		waitForProcessOutput(t, transport, size.want)
	}
}

func TestProcessTerminalOutputAndExit(t *testing.T) {
	transport, err := NewProcessTransport("/bin/sh", []string{"-c", `
printf 'STDOUT:%s\n' "$1"
printf 'STDERR\n' >&2
exit 7
`, "shellink-test", "argument with 'quotes' and spaces"}, nil, "xterm", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = transport.Close() })
	var output strings.Builder
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case chunk, ok := <-transport.Output():
			if !ok {
				goto done
			}
			output.WriteString(chunk)
		case <-timer.C:
			t.Fatalf("process output did not close: %q", output.String())
		}
	}
done:
	if got, want := output.String(), "STDOUT:argument with 'quotes' and spaces\r\nSTDERR\r\n"; got != want {
		t.Fatalf("terminal output = %q, want %q", got, want)
	}
	select {
	case err := <-transport.Done():
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
			t.Fatalf("process exit = %v, want exit code 7", err)
		}
	case <-timer.C:
		t.Fatal("process did not finish")
	}
}
