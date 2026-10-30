//go:build windows

package cli

import (
	"golang.org/x/sys/windows"
)

// enableConsoleColors turns on virtual terminal processing for stdout, which is
// what makes ANSI escapes render instead of being printed as garbage.
//
// Windows 10 consoles understand the sequences but ship with the mode off, and
// every older conhost ignored them outright; neither failure is fatal, so the
// best effort here matches what the interactive terminal does with raw mode.
func enableConsoleColors() {
	handle, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return
	}
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return
	}
	_ = windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
}
