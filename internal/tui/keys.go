package tui

// Key bindings for the TUI.
var keys = struct {
	Tab1    string
	Tab2    string
	Tab3    string
	Quit    string
	Up      string
	Down    string
	Toggle  string
	Reload  string
	Delete  string
	Test    string
	Enter   string
}{
	Tab1:   "1",
	Tab2:   "2",
	Tab3:   "3",
	Quit:   "q",
	Up:     "up",
	Down:   "down",
	Toggle: "s",
	Reload: "r",
	Delete: "d",
	Test:   "t",
	Enter:  "enter",
}

// HelpText returns the global keybinding help string.
func HelpText() string {
	return keyStyle.Render("[1/2/3]") + " tabs  " +
		keyStyle.Render("[s]") + " proxy  " +
		keyStyle.Render("[r]") + " reload  " +
		keyStyle.Render("[q]") + " quit"
}
