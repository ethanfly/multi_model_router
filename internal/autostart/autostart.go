package autostart

import (
	"fmt"
	"os"
	"os/exec"
)

const regKey = `HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
const appName = "MultiModelRouter"

// Enable adds the application to Windows startup registry.
func Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	return exec.Command("reg", "add", regKey, "/v", appName, "/t", "REG_SZ", "/d", exe, "/f").Run()
}

// Disable removes the application from Windows startup registry.
func Disable() error {
	return exec.Command("reg", "delete", regKey, "/v", appName, "/f").Run()
}

// IsEnabled checks if the application is in the Windows startup registry.
func IsEnabled() bool {
	return exec.Command("reg", "query", regKey, "/v", appName).Run() == nil
}
