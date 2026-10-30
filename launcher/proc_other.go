//go:build !windows

package main

// The launcher is distributed for Windows only; these stubs keep the package
// buildable (and testable) on other platforms.

func runningShellink() []int { return nil }

func alive(pid int) bool { return false }

func forceKill(pid int) {}

func isElevated() bool { return false }

func elevatedBrowser() bool { return false }
