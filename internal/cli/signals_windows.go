//go:build windows

package cli

import "os"

func platformSignals() []os.Signal { return []os.Signal{os.Interrupt} }
