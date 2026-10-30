//go:build !windows

package cli

// enableConsoleColors does nothing here: on Unix every terminal understands the
// sequences, and anything that does not is filtered out by the TTY test in
// useColor. It exists so the shared code has one name to call.
func enableConsoleColors() {}
