//go:build linux

package cli

import "golang.org/x/sys/unix"

const (
	terminalGetTermiosRequest = unix.TCGETS
	terminalSetTermiosRequest = unix.TCSETS
)
