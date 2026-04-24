//go:build !windows

package wintray

// MenuItem represents a tray menu item.
type MenuItem struct {
	ID      string
	Label   string
	Sep     bool
	Handler func()
}

// Run starts the system tray (no-op on non-Windows).
func Run(tooltip string, iconPNG []byte, onShow, onQuit func()) {
	// No-op on non-Windows platforms
}

// UpdateMenu updates the tray menu (no-op on non-Windows).
func UpdateMenu(items []MenuItem) {
	// No-op on non-Windows platforms
}

// Quit stops the system tray (no-op on non-Windows).
func Quit() {
	// No-op on non-Windows platforms
}
