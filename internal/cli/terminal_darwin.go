//go:build darwin

package cli

import "golang.org/x/sys/unix"

const (
	terminalGetTermiosRequest = unix.TIOCGETA
	terminalSetTermiosRequest = unix.TIOCSETA
)
