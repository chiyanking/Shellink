//go:build !windows

package main

import "errors"

// The launcher is distributed for Windows only; this stub keeps the package
// buildable on other platforms.
func openBrowser(url string) (string, error) {
	return "", errors.New("该平台不支持自动打开浏览器")
}
