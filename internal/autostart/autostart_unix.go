//go:build !windows

package autostart

import "errors"

// Enable adds the application to startup (not supported on non-Windows).
func Enable() error {
	return errors.New("autostart is only supported on Windows")
}

// Disable removes the application from startup (not supported on non-Windows).
func Disable() error {
	return errors.New("autostart is only supported on non-Windows")
}

// IsEnabled checks if the application is in the startup registry (always false on non-Windows).
func IsEnabled() bool {
	return false
}
