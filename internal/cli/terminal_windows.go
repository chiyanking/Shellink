//go:build windows

package cli

import (
	"fmt"
	"os"
	"strconv"

	"golang.org/x/sys/windows"
)

func enableRawTerminal() (func(), error) {
	input, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil {
		return nil, fmt.Errorf("get console input: %w", err)
	}
	output, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return nil, fmt.Errorf("get console output: %w", err)
	}
	var inputMode, outputMode uint32
	if err := windows.GetConsoleMode(input, &inputMode); err != nil {
		return nil, fmt.Errorf("interactive console input required: %w", err)
	}
	if err := windows.GetConsoleMode(output, &outputMode); err != nil {
		return nil, fmt.Errorf("interactive console output required: %w", err)
	}
	rawInput := inputMode &^ (windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT | windows.ENABLE_PROCESSED_INPUT)
	rawInput |= windows.ENABLE_VIRTUAL_TERMINAL_INPUT
	if err := windows.SetConsoleMode(input, rawInput); err != nil {
		return nil, fmt.Errorf("enable raw console input: %w", err)
	}
	if err := windows.SetConsoleMode(output, outputMode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		_ = windows.SetConsoleMode(input, inputMode)
		return nil, fmt.Errorf("enable ANSI console output: %w", err)
	}
	return func() {
		_ = windows.SetConsoleMode(input, inputMode)
		_ = windows.SetConsoleMode(output, outputMode)
	}, nil
}

func getTerminalDimensions() terminalDimensions {
	output, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err == nil {
		var info windows.ConsoleScreenBufferInfo
		if windows.GetConsoleScreenBufferInfo(output, &info) == nil {
			cols := int(info.Window.Right-info.Window.Left) + 1
			rows := int(info.Window.Bottom-info.Window.Top) + 1
			if cols > 0 && rows > 0 {
				return terminalDimensions{cols: cols, rows: rows}
			}
		}
	}
	cols, rows := 80, 24
	if value, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && value >= 20 {
		cols = value
	}
	if value, err := strconv.Atoi(os.Getenv("LINES")); err == nil && value >= 5 {
		rows = value
	}
	return terminalDimensions{cols: cols, rows: rows}
}
