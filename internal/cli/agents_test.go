package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentsConfigureDryRunDoesNotWriteFiles(t *testing.T) {
	home := t.TempDir()
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"agents", "configure",
		"--apps", "claude",
		"--home", home,
		"--port", "9680",
		"--api-key", "proxy-key",
		"--dry-run",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "claude") || !strings.Contains(out.String(), "DRY-RUN") {
		t.Fatalf("expected dry-run summary, got:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not write config, stat err=%v", err)
	}
}
