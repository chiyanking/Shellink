//go:build linux || darwin

package cli

import (
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

func enableRawTerminal() (func(), error) {
	fd := int(os.Stdin.Fd())
	original, err := unix.IoctlGetTermios(fd, terminalGetTermiosRequest)
	if err != nil {
		return nil, err
	}
	raw := *original
	raw.Iflag &^= unix.BRKINT | unix.ICRNL | unix.INPCK | unix.ISTRIP | unix.IXON
	raw.Oflag &^= unix.OPOST
	raw.Cflag |= unix.CS8
	raw.Lflag &^= unix.ECHO | unix.ICANON | unix.IEXTEN | unix.ISIG
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, terminalSetTermiosRequest, &raw); err != nil {
		return nil, err
	}
	return func() { _ = unix.IoctlSetTermios(fd, terminalSetTermiosRequest, original) }, nil
}

func getTerminalDimensions() terminalDimensions {
	if size, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ); err == nil && size.Col > 0 && size.Row > 0 {
		return terminalDimensions{cols: int(size.Col), rows: int(size.Row)}
	}
	cols, rows := terminalEnvDimensions()
	return terminalDimensions{cols: cols, rows: rows}
}

func terminalEnvDimensions() (int, int) {
	cols, rows := 80, 24
	if value, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && value >= 20 {
		cols = value
	}
	if value, err := strconv.Atoi(os.Getenv("LINES")); err == nil && value >= 5 {
		rows = value
	}
	return cols, rows
}
