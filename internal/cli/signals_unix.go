//go:build !windows

package cli

import (
	"os"
	"syscall"
)

func platformSignals() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM} }
