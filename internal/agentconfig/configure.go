package agentconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	json5 "github.com/yosuke-furukawa/json5/encoding/json5"
)

const (
	appClaude   = "claude"
	appCodex    = "codex"
	appGemini   = "gemini"
	appOpenCode = "opencode"
	appOpenClaw = "openclaw"

	providerID   = "multi_model_router"
	providerName = "Multi-Model Router"
)

type Options struct {
	HomeDir string
	Apps    []string
	Port    int
	BaseURL string
	APIKey  string
	Model   string
	DryRun  bool
}

type Result struct {
	Applied  bool      `json:"applied"`
	Changes  []Change  `json:"changes"`
	Warnings []Warning `json:"warnings"`
}

type Change struct {
	App        string `json:"app"`
	Path       string `json:"path"`
	Action     string `json:"action"`
	Changed    bool   `json:"changed"`
	BackupPath string `json:"backupPath,omitempty"`
}

type Warning struct {
	App     string `json:"app"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

func Configure(opts Options) (*Result, error) {
	normalized, err := normalizeOptions(opts)
	if err != nil {
		return nil, err
	}

	result := &Result{Applied: !normalized.DryRun}
	for _, app := range normalizeApps(normalized.Apps) {
		var changes []Change
		var warnings []Warning
		switch app {
		case appClaude:
			changes, warnings, err = configureClaude(normalized)
		case appCodex:
			changes, warnings, err = configureCodex(normalized)
		case appGemini:
			changes, warnings, err = configureGemini(normalized)
		case appOpenCode:
			changes, warnings, err = configureOpenCode(normalized)
		case appOpenClaw:
			changes, warnings, err = configureOpenClaw(normalized)
		default:
			err = fmt.Errorf("unsupported app %q", app)
		}
		if err != nil {
			result.Warnings = append(result.Warnings, Warning{App: app, Message: err.Error()})
			continue
		}
		result.Changes = append(result.Changes, changes...)
		result.Warnings = append(result.Warnings, warnings...)
	}

	return result, nil
}

func normalizeOptions(opts Options) (Options, error) {
	if opts.HomeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return opts, fmt.Errorf("resolve home dir: %w", err)
		}
		opts.HomeDir = home
	}
	if opts.Port == 0 && opts.BaseURL == "" {
		opts.Port = 9680
	}
	if opts.Model == "" {
		opts.Model = "auto"
	}
	if opts.APIKey == "" {
		opts.APIKey = "multi-model-router"
	}
	return opts, nil
}

func normalizeApps(apps []string) []string {
	if len(apps) == 0 {
		return []string{appClaude, appCodex, appGemini, appOpenCode, appOpenClaw}
	}

	seen := map[string]bool{}
	var out []string
	for _, app := range apps {
		for _, part := range strings.Split(app, ",") {
			name := strings.ToLower(strings.TrimSpace(part))
			if name == "" {
				continue
			}
			if name == "all" {
				for _, candidate := range []string{appClaude, appCodex, appGemini, appOpenCode, appOpenClaw} {
					if !seen[candidate] {
						seen[candidate] = true
						out = append(out, candidate)
					}
				}
				continue
			}
			if name == "claude-code" || name == "claude_code" {
				name = appClaude
			}
			if name == "gemini-cli" || name == "gemini_cli" {
				name = appGemini
			}
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	if len(out) == 0 {
		return []string{appClaude, appCodex, appGemini, appOpenCode, appOpenClaw}
	}
	return out
}

func baseURL(opts Options) string {
	if opts.BaseURL != "" {
		return strings.TrimRight(opts.BaseURL, "/")
	}
	return "http://127.0.0.1:" + strconv.Itoa(opts.Port)
}

func openAIBaseURL(opts Options) string {
	base := baseURL(opts)
	if strings.HasSuffix(base, "/v1") {
		return base
	}
	return base + "/v1"
}

func configureClaude(opts Options) ([]Change, []Warning, error) {
	path := filepath.Join(opts.HomeDir, ".claude", "settings.json")
	cfg, err := readJSONObject(path)
	if err != nil {
		return nil, nil, err
	}

	env := ensureObject(cfg, "env")
	env["ANTHROPIC_BASE_URL"] = baseURL(opts)
	env["ANTHROPIC_AUTH_TOKEN"] = opts.APIKey
	env["ANTHROPIC_API_KEY"] = opts.APIKey
	env["ANTHROPIC_MODEL"] = opts.Model
	env["ANTHROPIC_SMALL_FAST_MODEL"] = opts.Model

	change, err := writeJSONChange(appClaude, path, cfg, opts.DryRun)
	if err != nil {
		return nil, nil, err
	}
	return []Change{change}, nil, nil
}

func configureCodex(opts Options) ([]Change, []Warning, error) {
	authPath := filepath.Join(opts.HomeDir, ".codex", "auth.json")
	auth, err := readJSONObject(authPath)
	if err != nil {
		return nil, nil, err
	}
	auth["OPENAI_API_KEY"] = opts.APIKey

	configPath := filepath.Join(opts.HomeDir, ".codex", "config.toml")
	configText := ""
	if data, err := os.ReadFile(configPath); err == nil {
		configText = string(data)
	} else if !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("read %s: %w", configPath, err)
	}
	configText = setTopLevelTOMLString(configText, "model_provider", providerID)
	configText = setTopLevelTOMLString(configText, "model", opts.Model)
	configText = upsertTOMLTable(configText, "model_providers."+providerID, map[string]string{
		"name":     providerName,
		"base_url": openAIBaseURL(opts),
		"env_key":  "OPENAI_API_KEY",
		"wire_api": "chat",
	})

	authChange, err := writeJSONChange(appCodex, authPath, auth, opts.DryRun)
	if err != nil {
		return nil, nil, err
	}
	configChange, err := writeTextChange(appCodex, configPath, []byte(configText), opts.DryRun)
	if err != nil {
		return nil, nil, err
	}
	warnings := []Warning{{
		App:     appCodex,
		Path:    configPath,
		Message: "Codex is configured with wire_api=chat because this proxy exposes OpenAI chat/completions and Anthropic messages endpoints.",
	}}
	return []Change{authChange, configChange}, warnings, nil
}

func configureGemini(opts Options) ([]Change, []Warning, error) {
	path := filepath.Join(opts.HomeDir, ".gemini", ".env")
	env := map[string]string{}
	if data, err := os.ReadFile(path); err == nil {
		env = parseEnv(string(data))
	} else if !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	env["GEMINI_API_KEY"] = opts.APIKey
	env["GOOGLE_GEMINI_BASE_URL"] = baseURL(opts)
	env["GEMINI_MODEL"] = opts.Model

	envChange, err := writeTextChange(appGemini, path, []byte(serializeEnv(env)), opts.DryRun)
	if err != nil {
		return nil, nil, err
	}

	settingsPath := filepath.Join(opts.HomeDir, ".gemini", "settings.json")
	settings, err := readJSONObject(settingsPath)
	if err != nil {
		return nil, nil, err
	}
	security := ensureObject(settings, "security")
	auth := ensureObject(security, "auth")
	auth["selectedType"] = "gemini-api-key"
	settingsChange, err := writeJSONChange(appGemini, settingsPath, settings, opts.DryRun)
	if err != nil {
		return nil, nil, err
	}

	warnings := []Warning{{
		App:     appGemini,
		Path:    path,
		Message: "Gemini CLI may require a native Gemini-compatible API; this proxy currently exposes OpenAI-compatible and Anthropic-compatible APIs.",
	}}
	return []Change{envChange, settingsChange}, warnings, nil
}

func configureOpenCode(opts Options) ([]Change, []Warning, error) {
	path := filepath.Join(opts.HomeDir, ".config", "opencode", "opencode.json")
	cfg, err := readJSONObject(path)
	if err != nil {
		return nil, nil, err
	}
	if _, ok := cfg["$schema"]; !ok {
		cfg["$schema"] = "https://opencode.ai/config.json"
	}
	providers := ensureObject(cfg, "provider")
	providers[providerID] = map[string]any{
		"name": providerName,
		"npm":  "@ai-sdk/openai-compatible",
		"options": map[string]any{
			"baseURL": openAIBaseURL(opts),
			"apiKey":  opts.APIKey,
		},
		"models": map[string]any{
			opts.Model: map[string]any{"name": "Auto Route"},
			"race":     map[string]any{"name": "Race Route"},
		},
	}
	cfg["model"] = providerID + "/" + opts.Model

	change, err := writeJSONChange(appOpenCode, path, cfg, opts.DryRun)
	if err != nil {
		return nil, nil, err
	}
	return []Change{change}, nil, nil
}

func configureOpenClaw(opts Options) ([]Change, []Warning, error) {
	path := filepath.Join(opts.HomeDir, ".openclaw", "openclaw.json")
	cfg, err := readJSONObject(path)
	if err != nil {
		return nil, nil, err
	}
	models := ensureObject(cfg, "models")
	if _, ok := models["mode"]; !ok {
		models["mode"] = "merge"
	}
	providers := ensureObject(models, "providers")
	providers[providerID] = map[string]any{
		"baseUrl": openAIBaseURL(opts),
		"apiKey":  opts.APIKey,
		"api":     "openai-completions",
		"models": []any{
			map[string]any{"id": opts.Model, "name": "Auto Route"},
			map[string]any{"id": "race", "name": "Race Route"},
		},
	}
	agents := ensureObject(cfg, "agents")
	defaults := ensureObject(agents, "defaults")
	defaults["model"] = map[string]any{
		"primary":   providerID + "/" + opts.Model,
		"fallbacks": []any{providerID + "/race"},
	}

	change, err := writeJSONChange(appOpenClaw, path, cfg, opts.DryRun)
	if err != nil {
		return nil, nil, err
	}
	return []Change{change}, nil, nil
}

func readJSONObject(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		if json5Err := json5.Unmarshal(data, &out); json5Err != nil {
			return nil, fmt.Errorf("parse %s as JSON or JSON5: %w", path, err)
		}
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func ensureObject(parent map[string]any, key string) map[string]any {
	if existing, ok := parent[key].(map[string]any); ok {
		return existing
	}
	obj := map[string]any{}
	parent[key] = obj
	return obj
}

func writeJSONChange(app, path string, value map[string]any, dryRun bool) (Change, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return Change{}, fmt.Errorf("marshal %s config: %w", app, err)
	}
	data = append(data, '\n')
	return writeTextChange(app, path, data, dryRun)
}

func writeTextChange(app, path string, desired []byte, dryRun bool) (Change, error) {
	change := Change{App: app, Path: path, Action: "write"}
	current, err := os.ReadFile(path)
	if err == nil {
		if bytes.Equal(current, desired) {
			change.Action = "unchanged"
			return change, nil
		}
		change.Action = "update"
		change.Changed = true
	} else if os.IsNotExist(err) {
		change.Action = "create"
		change.Changed = true
	} else {
		return change, fmt.Errorf("read %s: %w", path, err)
	}

	if dryRun {
		return change, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return change, fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if len(current) > 0 {
		backupPath := backupFilePath(path)
		if err := os.WriteFile(backupPath, current, 0o644); err != nil {
			return change, fmt.Errorf("backup %s: %w", path, err)
		}
		change.BackupPath = backupPath
	}
	if err := atomicWrite(path, desired); err != nil {
		return change, err
	}
	return change, nil
}

func backupFilePath(path string) string {
	return fmt.Sprintf("%s.bak.%d", path, time.Now().UnixNano())
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	tmp := fmt.Sprintf("%s.tmp.%d", path, time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(path)
		if renameErr := os.Rename(tmp, path); renameErr != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("replace %s: %w", path, err)
		}
	}
	return nil
}

func parseEnv(content string) map[string]string {
	env := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		env[key] = strings.TrimSpace(value)
	}
	return env
}

func serializeEnv(env map[string]string) string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var lines []string
	for _, key := range keys {
		lines = append(lines, key+"="+env[key])
	}
	return strings.Join(lines, "\n") + "\n"
}

func setTopLevelTOMLString(input, key, value string) string {
	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	inTable := false
	replaced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inTable = true
		}
		if inTable || !strings.HasPrefix(trimmed, key+" ") && !strings.HasPrefix(trimmed, key+"=") {
			continue
		}
		lines[i] = key + " = " + quoteTOML(value)
		replaced = true
	}
	if !replaced {
		insertAt := 0
		for insertAt < len(lines) {
			trimmed := strings.TrimSpace(lines[insertAt])
			if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
				break
			}
			insertAt++
		}
		line := key + " = " + quoteTOML(value)
		lines = append(lines[:insertAt], append([]string{line}, lines[insertAt:]...)...)
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func upsertTOMLTable(input, table string, values map[string]string) string {
	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	header := "[" + table + "]"
	filtered := make([]string, 0, len(lines))
	inTarget := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inTarget = trimmed == header
			if inTarget {
				continue
			}
		}
		if inTarget {
			continue
		}
		filtered = append(filtered, line)
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	block := []string{"", header}
	for _, key := range keys {
		block = append(block, key+" = "+quoteTOML(values[key]))
	}
	filtered = append(filtered, block...)
	return strings.TrimRight(strings.Join(filtered, "\n"), "\n") + "\n"
}

func quoteTOML(value string) string {
	return strconv.Quote(value)
}
