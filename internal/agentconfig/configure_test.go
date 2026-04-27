package agentconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureDryRunReportsTargetsAndGeminiWarning(t *testing.T) {
	home := t.TempDir()

	result, err := Configure(Options{
		HomeDir: home,
		Apps:    []string{"all"},
		Port:    9680,
		APIKey:  "proxy-key",
		Model:   "auto",
		DryRun:  true,
	})
	if err != nil {
		t.Fatalf("dry-run configure failed: %v", err)
	}
	if result.Applied {
		t.Fatal("dry-run result must not be marked applied")
	}

	expectedApps := map[string]bool{
		"claude":   false,
		"codex":    false,
		"gemini":   false,
		"opencode": false,
		"openclaw": false,
	}
	for _, change := range result.Changes {
		if _, ok := expectedApps[change.App]; ok {
			expectedApps[change.App] = true
		}
	}
	for app, seen := range expectedApps {
		if !seen {
			t.Fatalf("expected dry-run change for %s in %#v", app, result.Changes)
		}
	}

	if len(result.Warnings) == 0 {
		t.Fatal("expected at least one warning")
	}
	foundGeminiWarning := false
	for _, warning := range result.Warnings {
		if warning.App == "gemini" && strings.Contains(strings.ToLower(warning.Message), "native gemini") {
			foundGeminiWarning = true
		}
	}
	if !foundGeminiWarning {
		t.Fatalf("expected Gemini native API warning, got %#v", result.Warnings)
	}

	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not create Claude config, stat err=%v", err)
	}
}

func TestConfigureApplyMergesConfigsAndCreatesBackups(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".claude", "settings.json"), `{"env":{"EXISTING":"1"},"permissions":{"allow":["Bash(ls)"]}}`)
	mustWrite(t, filepath.Join(home, ".codex", "auth.json"), `{"tokens":{"id":"keep"}}`)
	mustWrite(t, filepath.Join(home, ".codex", "config.toml"), "model = \"old\"\n[mcp_servers.keep]\ncommand = \"node\"\n")
	mustWrite(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{"plugin":["keep"],"provider":{"old":{"npm":"pkg"}}}`)
	mustWrite(t, filepath.Join(home, ".openclaw", "openclaw.json"), `{"tools":{"profile":"coding"},"models":{"mode":"merge","providers":{}}}`)
	mustWrite(t, filepath.Join(home, ".gemini", ".env"), "EXISTING=1\n")
	mustWrite(t, filepath.Join(home, ".gemini", "settings.json"), `{"ui":{"theme":"dark"},"security":{"auth":{"otherAuth":"keep"}}}`)

	result, err := Configure(Options{
		HomeDir: home,
		Apps:    []string{"claude", "codex", "gemini", "opencode", "openclaw"},
		Port:    9680,
		APIKey:  "proxy-key",
		Model:   "auto",
		DryRun:  false,
	})
	if err != nil {
		t.Fatalf("configure failed: %v", err)
	}
	if !result.Applied {
		t.Fatal("apply result must be marked applied")
	}

	claude := readJSON(t, filepath.Join(home, ".claude", "settings.json"))
	env := claude["env"].(map[string]any)
	if env["EXISTING"] != "1" {
		t.Fatalf("expected existing Claude env to be preserved, got %#v", env)
	}
	if env["ANTHROPIC_BASE_URL"] != "http://127.0.0.1:9680" {
		t.Fatalf("unexpected Claude base URL: %#v", env["ANTHROPIC_BASE_URL"])
	}
	if env["ANTHROPIC_MODEL"] != "auto" {
		t.Fatalf("unexpected Claude model: %#v", env["ANTHROPIC_MODEL"])
	}

	codexAuth := readJSON(t, filepath.Join(home, ".codex", "auth.json"))
	if codexAuth["OPENAI_API_KEY"] != "proxy-key" {
		t.Fatalf("expected Codex auth API key, got %#v", codexAuth)
	}
	codexConfig := mustRead(t, filepath.Join(home, ".codex", "config.toml"))
	for _, expected := range []string{
		`model = "auto"`,
		`model_provider = "multi_model_router"`,
		`[model_providers.multi_model_router]`,
		`base_url = "http://127.0.0.1:9680/v1"`,
		`wire_api = "chat"`,
		`[mcp_servers.keep]`,
	} {
		if !strings.Contains(codexConfig, expected) {
			t.Fatalf("expected Codex config to contain %q, got:\n%s", expected, codexConfig)
		}
	}

	opencode := readJSON(t, filepath.Join(home, ".config", "opencode", "opencode.json"))
	if _, ok := opencode["provider"].(map[string]any)["multi_model_router"]; !ok {
		t.Fatalf("expected OpenCode provider, got %#v", opencode)
	}
	if len(opencode["plugin"].([]any)) != 1 {
		t.Fatalf("expected OpenCode plugin list to be preserved, got %#v", opencode["plugin"])
	}

	openclaw := readJSON(t, filepath.Join(home, ".openclaw", "openclaw.json"))
	providers := openclaw["models"].(map[string]any)["providers"].(map[string]any)
	if _, ok := providers["multi_model_router"]; !ok {
		t.Fatalf("expected OpenClaw provider, got %#v", openclaw)
	}
	defaultModel := openclaw["agents"].(map[string]any)["defaults"].(map[string]any)["model"].(map[string]any)
	if defaultModel["primary"] != "multi_model_router/auto" {
		t.Fatalf("unexpected OpenClaw default model: %#v", defaultModel)
	}

	geminiEnv := mustRead(t, filepath.Join(home, ".gemini", ".env"))
	if !strings.Contains(geminiEnv, "EXISTING=1") || !strings.Contains(geminiEnv, "GEMINI_MODEL=auto") {
		t.Fatalf("expected Gemini env to be merged, got:\n%s", geminiEnv)
	}
	geminiSettings := readJSON(t, filepath.Join(home, ".gemini", "settings.json"))
	auth := geminiSettings["security"].(map[string]any)["auth"].(map[string]any)
	if auth["otherAuth"] != "keep" {
		t.Fatalf("expected Gemini auth fields to be preserved, got %#v", auth)
	}
	if auth["selectedType"] != "gemini-api-key" {
		t.Fatalf("expected Gemini selectedType to be gemini-api-key, got %#v", auth["selectedType"])
	}

	backups := 0
	for _, change := range result.Changes {
		if change.BackupPath == "" {
			continue
		}
		if _, err := os.Stat(change.BackupPath); err != nil {
			t.Fatalf("expected backup %s to exist: %v", change.BackupPath, err)
		}
		backups++
	}
	if backups < 5 {
		t.Fatalf("expected backups for existing files, got %d in %#v", backups, result.Changes)
	}
}

func TestConfigureOpenCodeAcceptsJSON5Config(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{
  // OpenCode allows JSON5 comments.
  provider: {
    existing: {
      npm: "keep",
    },
  },
}`)

	result, err := Configure(Options{
		HomeDir: home,
		Apps:    []string{"opencode"},
		Port:    9680,
		APIKey:  "proxy-key",
		Model:   "auto",
	})
	if err != nil {
		t.Fatalf("configure failed: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", result.Warnings)
	}

	opencode := readJSON(t, filepath.Join(home, ".config", "opencode", "opencode.json"))
	providers := opencode["provider"].(map[string]any)
	if _, ok := providers["existing"]; !ok {
		t.Fatalf("expected existing JSON5 provider to be preserved, got %#v", providers)
	}
	if _, ok := providers["multi_model_router"]; !ok {
		t.Fatalf("expected router provider, got %#v", providers)
	}
}

func mustWrite(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data := mustRead(t, path)
	var out map[string]any
	if err := json.Unmarshal([]byte(data), &out); err != nil {
		t.Fatalf("parse json %s: %v\n%s", path, err, data)
	}
	return out
}
