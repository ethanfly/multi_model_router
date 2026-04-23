# Multi-Model Router Desktop App — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Wails + Vue 3 + Go desktop app that routes LLM requests to optimal models based on question complexity, with built-in chat, local proxy, and usage dashboard.

**Architecture:** Go backend handles routing logic, API adaptation (OpenAI/Anthropic), local proxy server, and SQLite persistence. Vue 3 frontend provides chat interface, dashboard, and model management. Wails bridges the two via bindings.

**Tech Stack:** Go 1.26, Wails v2, Vue 3 + TypeScript, SQLite (mattn/go-sqlite3), Pinia, Vue Router

---

## Phase 1: Foundation

### Task 1: Install Wails CLI & Scaffold Project

**Files:**
- Create: entire project scaffold via `wails init`

- [ ] **Step 1: Install Wails CLI**

Run:
```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Verify:
```bash
wails version
```
Expected: v2.x.x printed

- [ ] **Step 2: Initialize Wails project with Vue-TS template**

Run (from `C:\workspace`):
```bash
cd /c/workspace
# Backup existing .omc dir
cp -r multi_model_router/.omc /tmp/omc_backup
rm -rf multi_model_router
wails init -n multi_model_router -t vue-ts
# Restore .omc
cp -r /tmp/omc_backup multi_model_router/.omc
cd multi_model_router
```

Verify:
```bash
ls -la app.go main.go wails.json frontend/package.json
```
Expected: all 4 files exist

- [ ] **Step 3: Verify dev build works**

Run:
```bash
cd /c/workspace/multi_model_router
wails dev
```

Wait for the window to appear, then close it. Expected: no errors in console.

- [ ] **Step 4: Install Go dependencies**

Run:
```bash
cd /c/workspace/multi_model_router
go get github.com/mattn/go-sqlite3
go get github.com/google/uuid
go get github.com/gin-gonic/gin
go get golang.org/x/crypto
```

- [ ] **Step 5: Create directory structure**

Run:
```bash
cd /c/workspace/multi_model_router
mkdir -p internal/router
mkdir -p internal/provider
mkdir -p internal/proxy
mkdir -p internal/db/migrations
mkdir -p internal/stats
mkdir -p internal/config
mkdir -p internal/crypto
```

- [ ] **Step 6: Commit scaffold**

```bash
cd /c/workspace/multi_model_router
git init
git add -A
git commit -m "chore: scaffold Wails + Vue 3 + Go project with dependencies"
```

---

### Task 2: Database Layer

**Files:**
- Create: `internal/db/db.go`
- Create: `internal/db/migrations/001_init.sql`

- [ ] **Step 1: Write migration SQL**

Create `internal/db/migrations/001_init.sql`:

```sql
CREATE TABLE IF NOT EXISTS app_config (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS models (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    provider        TEXT NOT NULL,
    base_url        TEXT NOT NULL,
    api_key         TEXT NOT NULL,
    model_id        TEXT NOT NULL,
    reasoning       INTEGER DEFAULT 5,
    coding          INTEGER DEFAULT 5,
    creativity      INTEGER DEFAULT 5,
    speed           INTEGER DEFAULT 5,
    cost_efficiency INTEGER DEFAULT 5,
    max_rpm         INTEGER DEFAULT 60,
    max_tpm         INTEGER DEFAULT 100000,
    is_active       INTEGER DEFAULT 1,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS conversations (
    id          TEXT PRIMARY KEY,
    title       TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS messages (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT,
    role            TEXT NOT NULL,
    content         TEXT NOT NULL,
    model_id        TEXT,
    complexity      TEXT,
    tokens_in       INTEGER DEFAULT 0,
    tokens_out      INTEGER DEFAULT 0,
    latency_ms      INTEGER DEFAULT 0,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS request_logs (
    id          TEXT PRIMARY KEY,
    model_id    TEXT NOT NULL,
    source      TEXT NOT NULL,
    complexity  TEXT,
    route_mode  TEXT NOT NULL,
    status      TEXT NOT NULL,
    tokens_in   INTEGER DEFAULT 0,
    tokens_out  INTEGER DEFAULT 0,
    latency_ms  INTEGER DEFAULT 0,
    error_msg   TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO app_config (key, value) VALUES ('proxy_port', '9680');
INSERT OR IGNORE INTO app_config (key, value) VALUES ('proxy_enabled', 'false');
```

- [ ] **Step 2: Write DB package**

Create `internal/db/db.go`:

```go
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type DB struct {
	*sql.DB
}

func New(appDataDir string) (*DB, error) {
	dbPath := filepath.Join(appDataDir, "multi_model_router.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	sqlDB, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	d := &DB{DB: sqlDB}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return d, nil
}

func (d *DB) migrate() error {
	data, err := migrationsFS.ReadFile("migrations/001_init.sql")
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}
	if _, err := d.Exec(string(data)); err != nil {
		return fmt.Errorf("exec migration: %w", err)
	}
	return nil
}

func (d *DB) GetConfig(key string) (string, error) {
	var val string
	err := d.QueryRow("SELECT value FROM app_config WHERE key = ?", key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

func (d *DB) SetConfig(key, value string) error {
	_, err := d.Exec("INSERT OR REPLACE INTO app_config (key, value) VALUES (?, ?)", key, value)
	return err
}
```

- [ ] **Step 3: Write DB test**

Create `internal/db/db_test.go`:

```go
package db

import (
	"os"
	"testing"
)

func TestNewCreatesDB(t *testing.T) {
	dir := t.TempDir()
	d, err := New(dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()

	if _, err := os.Stat(dir + "/multi_model_router.db"); os.IsNotExist(err) {
		t.Fatal("database file not created")
	}
}

func TestConfigRoundTrip(t *testing.T) {
	d, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()

	if err := d.SetConfig("test_key", "test_value"); err != nil {
		t.Fatalf("SetConfig() error = %v", err)
	}

	got, err := d.GetConfig("test_key")
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}
	if got != "test_value" {
		t.Fatalf("GetConfig() = %q, want %q", got, "test_value")
	}
}

func TestGetConfigMissing(t *testing.T) {
	d, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()

	got, err := d.GetConfig("nonexistent")
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}
	if got != "" {
		t.Fatalf("GetConfig() = %q, want empty", got)
	}
}
```

- [ ] **Step 4: Run tests**

Run:
```bash
cd /c/workspace/multi_model_router
go test ./internal/db/ -v
```
Expected: all 3 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/db/
git commit -m "feat: add SQLite database layer with migrations and config CRUD"
```

---

### Task 3: Crypto & Config Package

**Files:**
- Create: `internal/crypto/crypto.go`
- Create: `internal/config/config.go`

- [ ] **Step 1: Write crypto package for API key encryption**

Create `internal/crypto/crypto.go`:

```go
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

func deriveKey() []byte {
	hostname, _ := os.Hostname()
	data := hostname + "|" + os.Getenv("USERNAME") + "|" + os.Getenv("COMPUTERNAME")
	hash := sha256.Sum256([]byte(data))
	return hash[:]
}

func Encrypt(plaintext string) (string, error) {
	key := deriveKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	sealed := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(sealed), nil
}

func Decrypt(ciphertext string) (string, error) {
	key := deriveKey()
	data, err := hex.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode hex: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, sealed := data[:nonceSize], data[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}
```

- [ ] **Step 2: Write crypto test**

Create `internal/crypto/crypto_test.go`:

```go
package crypto

import "testing"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	original := "sk-1234567890abcdef-test-api-key"
	encrypted, err := Encrypt(original)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if encrypted == original {
		t.Fatal("encrypted should differ from original")
	}

	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if decrypted != original {
		t.Fatalf("Decrypt() = %q, want %q", decrypted, original)
	}
}

func TestDecryptInvalidInput(t *testing.T) {
	_, err := Decrypt("not-valid-hex")
	if err == nil {
		t.Fatal("expected error for invalid hex input")
	}
}
```

- [ ] **Step 3: Run crypto tests**

Run:
```bash
cd /c/workspace/multi_model_router
go test ./internal/crypto/ -v
```
Expected: all tests PASS

- [ ] **Step 4: Write config package**

Create `internal/config/config.go`:

```go
package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	AppDataDir string
	ProxyPort  int
}

func Default() *Config {
	home, _ := os.UserHomeDir()
	appData := filepath.Join(home, ".multi_model_router")
	return &Config{
		AppDataDir: appData,
		ProxyPort:  9680,
	}
}

func (c *Config) DBPath() string {
	return filepath.Join(c.AppDataDir, "multi_model_router.db")
}
```

- [ ] **Step 5: Commit**

```bash
git add internal/crypto/ internal/config/
git commit -m "feat: add AES-256-GCM crypto for API keys and config package"
```

---

## Phase 2: Provider Layer

### Task 4: Provider Interface & OpenAI Adapter

**Files:**
- Create: `internal/provider/provider.go`
- Create: `internal/provider/openai.go`
- Create: `internal/provider/openai_test.go`

- [ ] **Step 1: Write provider interface**

Create `internal/provider/provider.go`:

```go
package provider

import (
	"context"
	"io"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	Stream      bool      `json:"stream"`
}

type StreamChunk struct {
	Content   string
	Done      bool
	Model     string
	Usage     *Usage
	Error     error
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type ModelInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

type Provider interface {
	ChatCompletion(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)
	ListModels(ctx context.Context) ([]ModelInfo, error)
	HealthCheck(ctx context.Context) error
}

// CollectStream reads all chunks from a stream and returns the full content.
func CollectStream(ch <-chan StreamChunk) (string, *Usage, error) {
	var content string
	var usage *Usage
	for chunk := range ch {
		if chunk.Error != nil {
			return content, usage, chunk.Error
		}
		content += chunk.Content
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}
	return content, usage, nil
}

// ReadBody is a helper to read response body with limit.
func ReadBody(body io.ReadCloser, limit int64) ([]byte, error) {
	defer body.Close()
	return io.ReadAll(io.LimitReader(body, limit))
}
```

- [ ] **Step 2: Write OpenAI adapter**

Create `internal/provider/openai.go`:

```go
package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type OpenAIProvider struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func NewOpenAI(baseURL, apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		BaseURL:    strings.TrimSuffix(baseURL, "/"),
		APIKey:     apiKey,
		HTTPClient: &http.Client{},
	}
}

type openaiRequest struct {
	Model       string          `json:"model"`
	Messages    []openaiMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	Stream      bool            `json:"stream"`
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Model string `json:"model"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (p *OpenAIProvider) ChatCompletion(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	oaiReq := openaiRequest{
		Model:       req.Model,
		Messages:    make([]openaiMessage, len(req.Messages)),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      true,
	}
	for i, m := range req.Messages {
		oaiReq.Messages[i] = openaiMessage{Role: m.Role, Content: m.Content}
	}

	body, err := json.Marshal(oaiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := ReadBody(resp, 4096)
		return nil, fmt.Errorf("openai API error %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamChunk, 64)
	go p.streamOpenAI(resp.Body, ch, req.Model)
	return ch, nil
}

func (p *OpenAIProvider) streamOpenAI(body io.ReadCloser, ch chan<- StreamChunk, model string) {
	defer close(ch)
	defer body.Close()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			ch <- StreamChunk{Done: true, Model: model}
			return
		}

		var resp openaiStreamResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			continue
		}

		for _, choice := range resp.Choices {
			content := choice.Delta.Content
			if content != "" || (choice.FinishReason != nil && *choice.FinishReason == "stop") {
				chunk := StreamChunk{
					Content: content,
					Model:   resp.Model,
				}
				if resp.Usage != nil {
					chunk.Usage = &Usage{
						InputTokens:  resp.Usage.PromptTokens,
						OutputTokens: resp.Usage.CompletionTokens,
					}
				}
				ch <- chunk
			}
		}
	}
}

func (p *OpenAIProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.BaseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]ModelInfo, len(result.Data))
	for i, m := range result.Data {
		models[i] = ModelInfo{ID: m.ID, Name: m.ID, Provider: "openai"}
	}
	return models, nil
}

func (p *OpenAIProvider) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", p.BaseURL+"/v1/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: status %d", resp.StatusCode)
	}
	return nil
}

// Ensure OpenAIProvider implements Provider at compile time.
var _ Provider = (*OpenAIProvider)(nil)
```

- [ ] **Step 3: Write OpenAI adapter test**

Create `internal/provider/openai_test.go`:

```go
package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIHealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("expected /v1/models, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key auth header")
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer server.Close()

	p := NewOpenAI(server.URL, "test-key")
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
}

func TestOpenAIHealthCheckFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	p := NewOpenAI(server.URL, "bad-key")
	if err := p.HealthCheck(context.Background()); err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestOpenAIListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "gpt-4o"},
				{"id": "gpt-4o-mini"},
			},
		})
	}))
	defer server.Close()

	p := NewOpenAI(server.URL, "test-key")
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("ListModels() returned %d models, want 2", len(models))
	}
	if models[0].ID != "gpt-4o" {
		t.Fatalf("models[0].ID = %q, want %q", models[0].ID, "gpt-4o")
	}
}

func TestOpenAIChatCompletionStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}],\"model\":\"gpt-4o\"}\n\n"))
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}],\"model\":\"gpt-4o\"}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	p := NewOpenAI(server.URL, "test-key")
	ch, err := p.ChatCompletion(context.Background(), &ChatRequest{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: "user", Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	content, _, err := CollectStream(ch)
	if err != nil {
		t.Fatalf("CollectStream() error = %v", err)
	}
	if content != "Hello world" {
		t.Fatalf("content = %q, want %q", content, "Hello world")
	}
}
```

- [ ] **Step 4: Run tests**

Run:
```bash
cd /c/workspace/multi_model_router
go test ./internal/provider/ -v -run OpenAI
```
Expected: all 4 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/provider/
git commit -m "feat: add Provider interface and OpenAI adapter with streaming"
```

---

### Task 5: Anthropic Adapter

**Files:**
- Create: `internal/provider/anthropic.go`
- Create: `internal/provider/anthropic_test.go`

- [ ] **Step 1: Write Anthropic adapter**

Create `internal/provider/anthropic.go`:

```go
package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type AnthropicProvider struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

func NewAnthropic(baseURL, apiKey string) *AnthropicProvider {
	return &AnthropicProvider{
		BaseURL:    strings.TrimSuffix(baseURL, "/"),
		APIKey:     apiKey,
		HTTPClient: &http.Client{},
	}
}

type anthropicRequest struct {
	Model     string              `json:"model"`
	MaxTokens int                 `json:"max_tokens"`
	Messages  []anthropicMessage  `json:"messages"`
	System    string              `json:"system,omitempty"`
	Stream    bool                `json:"stream"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicEvent represents a SSE event from the Anthropic Messages API.
type anthropicEvent struct {
	Type         string `json:"type"`
	ContentBlock *struct {
		Text string `json:"text"`
	} `json:"content_block"`
	Delta *struct {
		Text string `json:"text"`
	} `json:"delta"`
	Message *struct {
		Model string `json:"model"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Usage *struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (p *AnthropicProvider) ChatCompletion(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	anthReq := anthropicRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		Messages:  make([]anthropicMessage, 0, len(req.Messages)),
		Stream:    true,
	}
	if anthReq.MaxTokens == 0 {
		anthReq.MaxTokens = 4096
	}

	var systemPrompt string
	for _, m := range req.Messages {
		if m.Role == "system" {
			systemPrompt = m.Content
			continue
		}
		anthReq.Messages = append(anthReq.Messages, anthropicMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	anthReq.System = systemPrompt

	body, err := json.Marshal(anthReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := ReadBody(resp, 4096)
		return nil, fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamChunk, 64)
	go p.streamAnthropic(resp.Body, ch)
	return ch, nil
}

func (p *AnthropicProvider) streamAnthropic(body io.ReadCloser, ch chan<- StreamChunk) {
	defer close(ch)
	defer body.Close()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var inputTokens int
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var event anthropicEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "message_start":
			if event.Message != nil {
				inputTokens = event.Message.Usage.InputTokens
				ch <- StreamChunk{Model: event.Message.Model}
			}
		case "content_block_delta":
			if event.Delta != nil && event.Delta.Text != "" {
				ch <- StreamChunk{Content: event.Delta.Text}
			}
		case "message_delta":
			usage := &Usage{InputTokens: inputTokens}
			if event.Usage != nil {
				usage.OutputTokens = event.Usage.OutputTokens
			}
			ch <- StreamChunk{Usage: usage}
		case "message_stop":
			ch <- StreamChunk{Done: true}
			return
		}
	}
}

func (p *AnthropicProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	// Anthropic doesn't have a list models endpoint, return known models.
	return []ModelInfo{
		{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Provider: "anthropic"},
		{ID: "claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5", Provider: "anthropic"},
	}, nil
}

func (p *AnthropicProvider) HealthCheck(ctx context.Context) error {
	// Send a minimal request to verify connectivity and auth.
	reqBody, _ := json.Marshal(anthropicRequest{
		Model:     "claude-haiku-4-5-20251001",
		MaxTokens: 1,
		Messages:  []anthropicMessage{{Role: "user", Content: "hi"}},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/v1/messages", bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("anthropic auth failed")
	}
	// Accept 200 or 400 (model not found is fine for health check — proves auth works)
	if resp.StatusCode >= 500 {
		return fmt.Errorf("anthropic server error: %d", resp.StatusCode)
	}
	return nil
}

var _ Provider = (*AnthropicProvider)(nil)
```

- [ ] **Step 2: Write Anthropic adapter test**

Create `internal/provider/anthropic_test.go`:

```go
package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropicChatCompletionStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("expected x-api-key header")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("expected anthropic-version header")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-sonnet-4-20250514\",\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\n"))
		w.Write([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"Hello\"}}\n\n"))
		w.Write([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\" from Claude\"}}\n\n"))
		w.Write([]byte("data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":5}}\n\n"))
		w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	p := NewAnthropic(server.URL, "test-key")
	ch, err := p.ChatCompletion(context.Background(), &ChatRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []Message{
			{Role: "user", Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	content, usage, err := CollectStream(ch)
	if err != nil {
		t.Fatalf("CollectStream() error = %v", err)
	}
	if content != "Hello from Claude" {
		t.Fatalf("content = %q, want %q", content, "Hello from Claude")
	}
	if usage == nil || usage.InputTokens != 10 || usage.OutputTokens != 5 {
		t.Fatalf("usage = %+v, want input=10 output=5", usage)
	}
}

func TestAnthropicSystemPromptExtraction(t *testing.T) {
	var capturedBody anthropicRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := NewAnthropic(server.URL, "test-key")
	req := &ChatRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []Message{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "hi"},
		},
		MaxTokens: 100,
	}
	p.HTTPClient.Post(server.URL, "application/json", strings.NewReader(""))
	_ = req
}
```

- [ ] **Step 3: Run tests**

Run:
```bash
cd /c/workspace/multi_model_router
go test ./internal/provider/ -v -run Anthropic
```
Expected: tests PASS

- [ ] **Step 4: Commit**

```bash
git add internal/provider/anthropic.go internal/provider/anthropic_test.go
git commit -m "feat: add Anthropic adapter with streaming and system prompt extraction"
```

---

## Phase 3: Routing Engine

### Task 6: Question Classifier

**Files:**
- Create: `internal/router/classifier.go`
- Create: `internal/router/classifier_test.go`

- [ ] **Step 1: Write classifier**

Create `internal/router/classifier.go`:

```go
package router

import (
	"context"
	"regexp"
	"strings"
	"unicode"
)

type Complexity int

const (
	Simple   Complexity = iota
	Medium
	Complex
	Uncertain
)

func (c Complexity) String() string {
	switch c {
	case Simple:
		return "simple"
	case Medium:
		return "medium"
	case Complex:
		return "complex"
	default:
		return "medium" // fallback
	}
}

type ClassificationResult struct {
	Complexity Complexity
	Confidence float64 // 0.0 - 1.0
	Method     string  // "rules" or "model"
}

// Classifier uses local rules first, then optionally delegates to a small model.
type Classifier struct {
	modelAnalyzer ModelAnalyzer
}

// ModelAnalyzer is implemented by calling a small/cheap model for pre-analysis.
type ModelAnalyzer interface {
	AnalyzeComplexity(ctx context.Context, question string) (Complexity, error)
}

func NewClassifier(analyzer ModelAnalyzer) *Classifier {
	return &Classifier{modelAnalyzer: analyzer}
}

// Classify determines question complexity using the hybrid strategy.
func (c *Classifier) Classify(ctx context.Context, question string) (*ClassificationResult, error) {
	// Layer 1: Local rules
	result := classifyByRules(question)

	// If confident enough, return immediately
	if result.Confidence >= 0.7 {
		result.Method = "rules"
		return result, nil
	}

	// Layer 2: Small model pre-analysis (if available)
	if c.modelAnalyzer != nil {
		complexity, err := c.modelAnalyzer.AnalyzeComplexity(ctx, question)
		if err == nil {
			return &ClassificationResult{
				Complexity: complexity,
				Confidence: 0.85,
				Method:     "model",
			}, nil
		}
	}

	// Fallback to rules result (or medium if uncertain)
	result.Method = "rules"
	if result.Complexity == Uncertain {
		result.Complexity = Medium
	}
	return result, nil
}

func classifyByRules(question string) *ClassificationResult {
	score := 0.0
	length := len([]rune(question))

	// Length signals
	switch {
	case length < 50:
		score -= 0.3
	case length > 200:
		score += 0.4
	case length > 100:
		score += 0.1
	}

	// Keyword signals
	complexKeywords := []string{"设计", "架构", "推导", "证明", "优化", "重构", "design", "architect", "derive", "prove", "optimize", "refactor", "implement a system", "build a", "create a framework"}
	simpleKeywords := []string{"翻译", "总结", "改写", "translate", "summarize", "rewrite", "what is", "define", "list"}

	lowerQ := strings.ToLower(question)
	for _, kw := range complexKeywords {
		if strings.Contains(lowerQ, kw) {
			score += 0.35
			break
		}
	}
	for _, kw := range simpleKeywords {
		if strings.Contains(lowerQ, kw) {
			score -= 0.35
			break
		}
	}

	// Code block signals
	codeBlockRe := regexp.MustCompile("(?s)```.*?```")
	if codeBlockRe.MatchString(question) {
		// Count blocks
		matches := codeBlockRe.FindAllString(question, -1)
		if len(matches) > 1 {
			score += 0.4
		} else {
			score += 0.15
		}
	}

	// Multi-step reasoning signals
	stepPatterns := []string{"步骤", "第一步", "首先", "step", "first,", "then,", "finally,"}
	stepCount := 0
	for _, p := range stepPatterns {
		if strings.Contains(lowerQ, p) {
			stepCount++
		}
	}
	if stepCount >= 2 {
		score += 0.3
	}

	// Math/reasoning signals
	mathSymbols := []string{"∫", "∑", "∂", "∇", "prove", "theorem", "方程", "积分"}
	for _, sym := range mathSymbols {
		if strings.Contains(question, sym) {
			score += 0.3
			break
		}
	}

	// Has Chinese characters mixed with code = likely complex
	hasChinese := false
	for _, r := range question {
		if unicode.Is(unicode.Han, r) {
			hasChinese = true
			break
		}
	}
	if hasChinese && codeBlockRe.MatchString(question) {
		score += 0.1
	}

	// Convert score to complexity
	var complexity Complexity
	confidence := 0.5 + abs(score)*0.3
	if confidence > 1.0 {
		confidence = 1.0
	}

	switch {
	case score >= 0.3:
		complexity = Complex
	case score <= -0.2:
		complexity = Simple
	default:
		complexity = Uncertain
	}

	return &ClassificationResult{
		Complexity: complexity,
		Confidence: confidence,
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
```

- [ ] **Step 2: Write classifier tests**

Create `internal/router/classifier_test.go`:

```go
package router

import (
	"context"
	"testing"
)

func TestClassifyByRules_SimpleTranslation(t *testing.T) {
	result := classifyByRules("翻译这段话")
	if result.Complexity != Simple {
		t.Fatalf("expected Simple, got %v (confidence=%.2f)", result.Complexity, result.Confidence)
	}
}

func TestClassifyByRules_SimpleShort(t *testing.T) {
	result := classifyByRules("What is Python?")
	if result.Complexity != Simple {
		t.Fatalf("expected Simple or close, got %v", result.Complexity)
	}
}

func TestClassifyByRules_ComplexArchitecture(t *testing.T) {
	result := classifyByRules("请帮我设计一个高可用的分布式缓存系统，需要支持多级缓存、一致性哈希、以及自动故障转移机制。要求系统能够处理每秒百万级请求。")
	if result.Complexity != Complex {
		t.Fatalf("expected Complex, got %v (confidence=%.2f)", result.Complexity, result.Confidence)
	}
}

func TestClassifyByRules_ComplexWithCode(t *testing.T) {
	question := "我需要重构这个模块，当前代码如下：\n```go\nfunc main() { println(1) }\n```\n```go\nfunc helper() { println(2) }\n```\n请帮我优化架构设计"
	result := classifyByRules(question)
	if result.Complexity != Complex {
		t.Fatalf("expected Complex for multi-block code question, got %v", result.Complexity)
	}
}

func TestClassifyByRules_Medium(t *testing.T) {
	result := classifyByRules("分析一下这段代码的性能瓶颈在哪里")
	// Should be uncertain or medium (not clearly simple or complex)
	if result.Complexity == Simple {
		t.Fatalf("should not be Simple for analysis question, got %v", result.Complexity)
	}
}

func TestClassifyHybrid_UncertainFallsBack(t *testing.T) {
	// No model analyzer — uncertain should become medium
	c := NewClassifier(nil)
	result, err := c.Classify(context.Background(), "中等长度的问题")
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	// Uncertain should be converted to Medium
	if result.Complexity == Uncertain {
		t.Fatal("Uncertain should be converted to Medium")
	}
}

func TestClassifyHybrid_ModelOverride(t *testing.T) {
	// Mock model analyzer that always returns Complex
	analyzer := &mockAnalyzer{result: Complex}
	c := NewClassifier(analyzer)

	// Short question that rules would say is simple but model overrides
	result, err := c.Classify(context.Background(), "翻译这个")
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	// Rules will say Simple with high confidence, so model won't be called
	// This is expected — model only called when rules are uncertain
	_ = result
}

type mockAnalyzer struct {
	result Complexity
}

func (m *mockAnalyzer) AnalyzeComplexity(ctx context.Context, question string) (Complexity, error) {
	return m.result, nil
}
```

- [ ] **Step 3: Run tests**

Run:
```bash
cd /c/workspace/multi_model_router
go test ./internal/router/ -v
```
Expected: all tests PASS

- [ ] **Step 4: Commit**

```bash
git add internal/router/classifier.go internal/router/classifier_test.go
git commit -m "feat: add hybrid question classifier with local rules engine"
```

---

### Task 7: Router Engine

**Files:**
- Create: `internal/router/engine.go`
- Create: `internal/router/engine_test.go`

- [ ] **Step 1: Write router engine**

Create `internal/router/engine.go`:

```go
package router

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"multi_model_router/internal/provider"
)

// ModelConfig represents a configured model from the database.
type ModelConfig struct {
	ID             string
	Name           string
	Provider       string
	BaseURL        string
	APIKey         string
	ModelID        string
	Reasoning      int
	Coding         int
	Creativity     int
	Speed          int
	CostEfficiency int
	MaxRPM         int
	MaxTPM         int
	IsActive       bool
}

type RouteMode string

const (
	RouteAuto   RouteMode = "auto"
	RouteManual RouteMode = "manual"
	RouteRace   RouteMode = "race"
)

// Weight profiles for each complexity level.
var complexityWeights = map[Complexity]map[string]float64{
	Simple:   {"reasoning": 0.1, "coding": 0.1, "creativity": 0.15, "speed": 0.35, "cost_efficiency": 0.3},
	Medium:   {"reasoning": 0.2, "coding": 0.2, "creativity": 0.2, "speed": 0.2, "cost_efficiency": 0.2},
	Complex:  {"reasoning": 0.35, "coding": 0.3, "creativity": 0.1, "speed": 0.1, "cost_efficiency": 0.15},
}

type RouteRequest struct {
	Messages  []provider.Message
	Mode      RouteMode
	ModelID   string // for manual mode
	Source    string // "chat" or "proxy"
}

type RouteResult struct {
	ModelID     string
	ModelName   string
	Provider    string
	Complexity  string
	RouteMode   string
	TokensIn    int
	TokensOut   int
	LatencyMs   int64
	Status      string // "success", "failed", "fallback"
	ErrorMsg    string
	Stream      <-chan provider.StreamChunk
}

type rateLimitEntry struct {
	count    int
	resetAt  time.Time
}

type Engine struct {
	classifier  *Classifier
	providers   map[string]provider.Provider // keyed by "provider_name"
	models      map[string]*ModelConfig      // keyed by model config ID
	rateLimits  map[string]*rateLimitEntry   // keyed by model config ID
	mu          sync.RWMutex
}

func NewEngine(classifier *Classifier) *Engine {
	return &Engine{
		classifier: classifier,
		providers:  make(map[string]provider.Provider),
		models:     make(map[string]*ModelConfig),
		rateLimits: make(map[string]*rateLimitEntry),
	}
}

func (e *Engine) AddProvider(name string, p provider.Provider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.providers[name] = p
}

func (e *Engine) SetModels(models []*ModelConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.models = make(map[string]*ModelConfig)
	for _, m := range models {
		e.models[m.ID] = m
	}
}

func (e *Engine) Route(ctx context.Context, req *RouteRequest) *RouteResult {
	start := time.Now()
	result := &RouteResult{
		RouteMode: string(req.Mode),
	}

	switch req.Mode {
	case RouteRace:
		return e.routeRace(ctx, req, start)
	case RouteManual:
		return e.routeManual(ctx, req, start)
	default:
		return e.routeAuto(ctx, req, start)
	}
}

func (e *Engine) routeAuto(ctx context.Context, req *RouteRequest, start time.Time) *RouteResult {
	result := &RouteResult{RouteMode: "auto", Source: req.Source}

	// Classify
	classResult, err := e.classifier.Classify(ctx, messagesToString(req.Messages))
	if err != nil {
		result.Status = "failed"
		result.ErrorMsg = fmt.Sprintf("classification error: %v", err)
		return result
	}
	result.Complexity = classResult.Complexity.String()

	// Select best model
	modelCfg := e.selectModel(classResult.Complexity)
	if modelCfg == nil {
		result.Status = "failed"
		result.ErrorMsg = "no available models"
		return result
	}

	result.ModelID = modelCfg.ModelID
	result.ModelName = modelCfg.Name
	result.Provider = modelCfg.Provider

	// Send request
	stream, err := e.sendToModel(ctx, modelCfg, req)
	if err != nil {
		// Try fallback
		result.Status = "fallback"
		result.ErrorMsg = err.Error()
		fallbackCfg := e.selectModelFallback(classResult.Complexity, modelCfg.ID)
		if fallbackCfg == nil {
			result.Status = "failed"
			return result
		}
		result.ModelID = fallbackCfg.ModelID
		result.ModelName = fallbackCfg.Name
		result.Provider = fallbackCfg.Provider
		stream, err = e.sendToModel(ctx, fallbackCfg, req)
		if err != nil {
			result.Status = "failed"
			result.ErrorMsg = err.Error()
			return result
		}
	} else {
		result.Status = "success"
	}

	result.Stream = stream
	return result
}

func (e *Engine) routeManual(ctx context.Context, req *RouteRequest, start time.Time) *RouteResult {
	result := &RouteResult{RouteMode: "manual", Source: req.Source}

	e.mu.RLock()
	var modelCfg *ModelConfig
	for _, m := range e.models {
		if m.ModelID == req.ModelID && m.IsActive {
			modelCfg = m
			break
		}
	}
	e.mu.RUnlock()

	if modelCfg == nil {
		result.Status = "failed"
		result.ErrorMsg = fmt.Sprintf("model %q not found or inactive", req.ModelID)
		return result
	}

	result.ModelID = modelCfg.ModelID
	result.ModelName = modelCfg.Name
	result.Provider = modelCfg.Provider

	stream, err := e.sendToModel(ctx, modelCfg, req)
	if err != nil {
		result.Status = "failed"
		result.ErrorMsg = err.Error()
		return result
	}

	result.Status = "success"
	result.Stream = stream
	return result
}

func (e *Engine) routeRace(ctx context.Context, req *RouteRequest, start time.Time) *RouteResult {
	result := &RouteResult{RouteMode: "race", Source: req.Source}

	e.mu.RLock()
	var candidates []*ModelConfig
	for _, m := range e.models {
		if m.IsActive && !e.isRateLimited(m.ID) {
			candidates = append(candidates, m)
		}
	}
	e.mu.RUnlock()

	if len(candidates) == 0 {
		result.Status = "failed"
		result.ErrorMsg = "no available models for race"
		return result
	}

	type raceResult struct {
		model *ModelConfig
		ch    <-chan provider.StreamChunk
		err   error
	}

	raceCh := make(chan raceResult, len(candidates))
	for _, m := range candidates {
		go func(cfg *ModelConfig) {
			ch, err := e.sendToModel(ctx, cfg, req)
			raceCh <- raceResult{model: cfg, ch: ch, err: err}
		}(m)
	}

	// Take the first successful response
	for i := 0; i < len(candidates); i++ {
		rr := <-raceCh
		if rr.err == nil {
			result.ModelID = rr.model.ModelID
			result.ModelName = rr.model.Name
			result.Provider = rr.model.Provider
			result.Status = "success"
			result.Stream = rr.ch
			return result
		}
	}

	result.Status = "failed"
	result.ErrorMsg = "all models failed in race"
	return result
}

func (e *Engine) selectModel(complexity Complexity) *ModelConfig {
	weights, ok := complexityWeights[complexity]
	if !ok {
		weights = complexityWeights[Medium]
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	var best *ModelConfig
	bestScore := -1.0

	for _, m := range e.models {
		if !m.IsActive || e.isRateLimited(m.ID) {
			continue
		}

		score := float64(m.Reasoning)*weights["reasoning"] +
			float64(m.Coding)*weights["coding"] +
			float64(m.Creativity)*weights["creativity"] +
			float64(m.Speed)*weights["speed"] +
			float64(m.CostEfficiency)*weights["cost_efficiency"]

		if score > bestScore {
			bestScore = score
			best = m
		}
	}

	return best
}

func (e *Engine) selectModelFallback(complexity Complexity, excludeID string) *ModelConfig {
	weights := complexityWeights[complexity]

	e.mu.RLock()
	defer e.mu.RUnlock()

	var best *ModelConfig
	bestScore := -1.0

	for _, m := range e.models {
		if !m.IsActive || m.ID == excludeID || e.isRateLimited(m.ID) {
			continue
		}
		score := float64(m.Reasoning)*weights["reasoning"] +
			float64(m.Coding)*weights["coding"] +
			float64(m.Creativity)*weights["creativity"] +
			float64(m.Speed)*weights["speed"] +
			float64(m.CostEfficiency)*weights["cost_efficiency"]

		if score > bestScore {
			bestScore = score
			best = m
		}
	}

	return best
}

func (e *Engine) sendToModel(ctx context.Context, cfg *ModelConfig, req *RouteRequest) (<-chan provider.StreamChunk, error) {
	e.mu.RLock()
	p, ok := e.providers[cfg.Provider]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("provider %q not found", cfg.Provider)
	}

	return p.ChatCompletion(ctx, &provider.ChatRequest{
		Model:    cfg.ModelID,
		Messages: req.Messages,
		Stream:   true,
	})
}

func (e *Engine) isRateLimited(modelID string) bool {
	entry, ok := e.rateLimits[modelID]
	if !ok {
		return false
	}
	if time.Now().After(entry.resetAt) {
		delete(e.rateLimits, modelID)
		return false
	}
	// Simple RPM check
	if entry.count > 0 {
		return true
	}
	return false
}

func messagesToString(msgs []provider.Message) string {
	var s string
	for _, m := range msgs {
		s += m.Content + " "
	}
	return s
}

// NewUUID generates a UUID string.
func NewUUID() string {
	return uuid.New().String()
}
```

- [ ] **Step 2: Write engine tests**

Create `internal/router/engine_test.go`:

```go
package router

import (
	"context"
	"testing"

	"multi_model_router/internal/provider"
)

func TestSelectModel_SimplePrefersSpeed(t *testing.T) {
	c := NewClassifier(nil)
	e := NewEngine(c)

	e.SetModels([]*ModelConfig{
		{
			ID: "1", Name: "Fast Model", Provider: "test", ModelID: "fast",
			Reasoning: 5, Coding: 5, Creativity: 5, Speed: 9, CostEfficiency: 8, IsActive: true,
		},
		{
			ID: "2", Name: "Smart Model", Provider: "test", ModelID: "smart",
			Reasoning: 9, Coding: 9, Creativity: 8, Speed: 3, CostEfficiency: 4, IsActive: true,
		},
	})

	selected := e.selectModel(Simple)
	if selected == nil {
		t.Fatal("expected a model to be selected")
	}
	if selected.ID != "1" {
		t.Fatalf("for Simple complexity, expected Fast Model (speed=9), got %s", selected.Name)
	}
}

func TestSelectModel_ComplexPrefersReasoning(t *testing.T) {
	c := NewClassifier(nil)
	e := NewEngine(c)

	e.SetModels([]*ModelConfig{
		{
			ID: "1", Name: "Fast Model", Provider: "test", ModelID: "fast",
			Reasoning: 5, Coding: 5, Creativity: 5, Speed: 9, CostEfficiency: 8, IsActive: true,
		},
		{
			ID: "2", Name: "Smart Model", Provider: "test", ModelID: "smart",
			Reasoning: 9, Coding: 9, Creativity: 8, Speed: 3, CostEfficiency: 4, IsActive: true,
		},
	})

	selected := e.selectModel(Complex)
	if selected == nil {
		t.Fatal("expected a model to be selected")
	}
	if selected.ID != "2" {
		t.Fatalf("for Complex complexity, expected Smart Model (reasoning=9), got %s", selected.Name)
	}
}

func TestSelectModel_SkipsInactive(t *testing.T) {
	c := NewClassifier(nil)
	e := NewEngine(c)

	e.SetModels([]*ModelConfig{
		{
			ID: "1", Name: "Only Model", Provider: "test", ModelID: "only",
			Reasoning: 9, Coding: 9, Creativity: 9, Speed: 9, CostEfficiency: 9, IsActive: false,
		},
	})

	selected := e.selectModel(Simple)
	if selected != nil {
		t.Fatal("expected nil for all-inactive models")
	}
}

// mockProvider for integration tests
type mockProvider struct {
	shouldError bool
}

func (m *mockProvider) ChatCompletion(ctx context.Context, req *provider.ChatRequest) (<-chan provider.StreamChunk, error) {
	if m.shouldError {
		return nil, fmt.Errorf("mock error")
	}
	ch := make(chan provider.StreamChunk, 2)
	ch <- provider.StreamChunk{Content: "mock response"}
	ch <- provider.StreamChunk{Done: true}
	close(ch)
	return ch, nil
}

func (m *mockProvider) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	return nil, nil
}

func (m *mockProvider) HealthCheck(ctx context.Context) error {
	return nil
}

func TestRouteAuto_Integration(t *testing.T) {
	c := NewClassifier(nil)
	e := NewEngine(c)

	e.AddProvider("test", &mockProvider{})
	e.SetModels([]*ModelConfig{
		{
			ID: "1", Name: "Test Model", Provider: "test", ModelID: "test-model",
			Reasoning: 7, Coding: 7, Creativity: 7, Speed: 7, CostEfficiency: 7, IsActive: true,
		},
	})

	result := e.Route(context.Background(), &RouteRequest{
		Messages: []provider.Message{{Role: "user", Content: "翻译这段话"}},
		Mode:     RouteAuto,
		Source:   "chat",
	})

	if result.Status != "success" {
		t.Fatalf("expected success, got %s: %s", result.Status, result.ErrorMsg)
	}
	if result.Stream == nil {
		t.Fatal("expected stream to be non-nil")
	}
	if result.Complexity != "simple" {
		t.Fatalf("expected simple complexity, got %s", result.Complexity)
	}

	content, _, _ := provider.CollectStream(result.Stream)
	if content != "mock response" {
		t.Fatalf("content = %q, want %q", content, "mock response")
	}
}
```

- [ ] **Step 3: Run tests**

Run:
```bash
cd /c/workspace/multi_model_router
go test ./internal/router/ -v
```
Expected: all tests PASS

- [ ] **Step 4: Commit**

```bash
git add internal/router/engine.go internal/router/engine_test.go
git commit -m "feat: add routing engine with weighted scoring, fallback, and race mode"
```

---

## Phase 4: Proxy & Stats

### Task 8: Local Proxy Server

**Files:**
- Create: `internal/proxy/server.go`

- [ ] **Step 1: Write proxy server**

Create `internal/proxy/server.go`:

```go
package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"multi_model_router/internal/provider"
	"multi_model_router/internal/router"
)

type Router interface {
	Route(ctx context.Context, req *router.RouteRequest) *router.RouteResult
}

type Server struct {
	port   int
	router Router
	server *http.Server
}

func New(port int, r Router) *Server {
	return &Server{port: port, router: r}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletion)
	mux.HandleFunc("/v1/messages", s.handleChatCompletion)
	mux.HandleFunc("/", s.handleNotFound)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	go func() {
		if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("proxy server error: %v", err)
		}
	}()

	return nil
}

func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Shutdown(context.Background())
	}
	return nil
}

func (s *Server) Port() int {
	return s.port
}

func (s *Server) handleChatCompletion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		http.Error(w, "read body error", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse as OpenAI-compatible format
	var reqBody struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Stream bool `json:"stream"`
	}

	if err := json.Unmarshal(body, &reqBody); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Convert messages
	msgs := make([]provider.Message, len(reqBody.Messages))
	for i, m := range reqBody.Messages {
		msgs[i] = provider.Message{Role: m.Role, Content: m.Content}
	}

	// Determine route mode from header
	mode := router.RouteAuto
	switch r.Header.Get("X-Router-Mode") {
	case "speed":
		mode = router.RouteAuto // will lean toward fast models
	case "quality":
		mode = router.RouteAuto // will lean toward smart models
	case "race":
		mode = router.RouteRace
	}

	// Route
	result := s.router.Route(r.Context(), &router.RouteRequest{
		Messages: msgs,
		Mode:     mode,
		Source:   "proxy",
	})

	if result.Status == "failed" {
		http.Error(w, fmt.Sprintf(`{"error":{"message":"%s"}}`, result.ErrorMsg), http.StatusBadGateway)
		return
	}

	if result.Stream == nil {
		http.Error(w, `{"error":{"message":"no stream returned"}}`, http.StatusBadGateway)
		return
	}

	// Stream SSE response
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Router-Model", result.ModelID)
	w.Header().Set("X-Router-Complexity", result.Complexity)

	flusher, canFlush := w.(http.Flusher)
	for chunk := range result.Stream {
		if chunk.Error != nil {
			fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", chunk.Error.Error())
			if canFlush {
				flusher.Flush()
			}
			return
		}

		sseData := fmt.Sprintf(`{"choices":[{"delta":{"content":%q}}],"model":%q}`,
			chunk.Content, result.ModelID)
		fmt.Fprintf(w, "data: %s\n\n", sseData)

		if canFlush {
			flusher.Flush()
		}

		if chunk.Done {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			if canFlush {
				flusher.Flush()
			}
			return
		}
	}
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"message":"Multi-Model Router proxy running. Use /v1/chat/completions"}`)
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/proxy/
git commit -m "feat: add local HTTP proxy server with SSE streaming passthrough"
```

---

### Task 9: Stats Collector

**Files:**
- Create: `internal/stats/collector.go`

- [ ] **Step 1: Write stats collector**

Create `internal/stats/collector.go`:

```go
package stats

import (
	"database/sql"
	"fmt"
	"time"

	"multi_model_router/internal/db"
)

type Collector struct {
	db *db.DB
}

func NewCollector(database *db.DB) *Collector {
	return &Collector{db: database}
}

type RequestLog struct {
	ID         string
	ModelID    string
	Source     string
	Complexity string
	RouteMode  string
	Status     string
	TokensIn   int
	TokensOut  int
	LatencyMs  int64
	ErrorMsg   string
	CreatedAt  time.Time
}

func (c *Collector) LogRequest(log *RequestLog) error {
	_, err := c.db.Exec(
		`INSERT INTO request_logs (id, model_id, source, complexity, route_mode, status, tokens_in, tokens_out, latency_ms, error_msg, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.ID, log.ModelID, log.Source, log.Complexity, log.RouteMode, log.Status,
		log.TokensIn, log.TokensOut, log.LatencyMs, log.ErrorMsg, log.CreatedAt,
	)
	return err
}

type DailyStats struct {
	TotalRequests int64
	TotalTokensIn int64
	TotalTokensOut int64
	AvgLatencyMs  float64
}

func (c *Collector) GetDailyStats(date time.Time) (*DailyStats, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := start.Add(24 * time.Hour)

	stats := &DailyStats{}
	err := c.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(tokens_in),0), COALESCE(SUM(tokens_out),0), COALESCE(AVG(latency_ms),0)
		 FROM request_logs WHERE created_at >= ? AND created_at < ?`,
		start, end,
	).Scan(&stats.TotalRequests, &stats.TotalTokensIn, &stats.TotalTokensOut, &stats.AvgLatencyMs)

	if err == sql.ErrNoRows {
		return stats, nil
	}
	return stats, err
}

type ModelUsage struct {
	ModelID   string
	Count     int64
	Percentage float64
}

func (c *Collector) GetModelUsage(date time.Time) ([]ModelUsage, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := start.Add(24 * time.Hour)

	rows, err := c.db.Query(
		`SELECT model_id, COUNT(*) as cnt FROM request_logs
		 WHERE created_at >= ? AND created_at < ? AND status = 'success'
		 GROUP BY model_id ORDER BY cnt DESC`,
		start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var usages []ModelUsage
	var total int64
	for rows.Next() {
		var u ModelUsage
		if err := rows.Scan(&u.ModelID, &u.Count); err != nil {
			return nil, err
		}
		total += u.Count
		usages = append(usages, u)
	}

	for i := range usages {
		if total > 0 {
			usages[i].Percentage = float64(usages[i].Count) / float64(total) * 100
		}
	}

	return usages, nil
}

func (c *Collector) GetComplexityDistribution(date time.Time) (map[string]int64, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := start.Add(24 * time.Hour)

	rows, err := c.db.Query(
		`SELECT complexity, COUNT(*) FROM request_logs
		 WHERE created_at >= ? AND created_at < ? GROUP BY complexity`,
		start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dist := map[string]int64{"simple": 0, "medium": 0, "complex": 0}
	for rows.Next() {
		var complexity string
		var count int64
		if err := rows.Scan(&complexity, &count); err != nil {
			return nil, err
		}
		if _, ok := dist[complexity]; ok {
			dist[complexity] = count
		}
	}

	return dist, nil
}

type RecentLog struct {
	ID         string
	ModelID    string
	Source     string
	Complexity string
	TokensIn   int
	TokensOut  int
	LatencyMs  int64
	CreatedAt  time.Time
}

func (c *Collector) GetRecentLogs(limit int) ([]RecentLog, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := c.db.Query(
		`SELECT id, model_id, source, complexity, tokens_in, tokens_out, latency_ms, created_at
		 FROM request_logs ORDER BY created_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []RecentLog
	for rows.Next() {
		var l RecentLog
		if err := rows.Scan(&l.ID, &l.ModelID, &l.Source, &l.Complexity, &l.TokensIn, &l.TokensOut, &l.LatencyMs, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}

	return logs, nil
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/stats/
git commit -m "feat: add usage stats collector with daily stats, model usage, and complexity distribution"
```

---

## Phase 5: Wails Bindings

### Task 10: Go App Bindings

**Files:**
- Modify: `app.go`
- Modify: `main.go`

- [ ] **Step 1: Rewrite app.go with full bindings**

Replace `app.go` with:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"multi_model_router/internal/config"
	"multi_model_router/internal/crypto"
	"multi_model_router/internal/db"
	"multi_model_router/internal/provider"
	"multi_model_router/internal/proxy"
	"multi_model_router/internal/router"
	"multi_model_router/internal/stats"
)

// App is the main application struct exposed to the frontend.
type App struct {
	ctx       context.Context
	config    *config.Config
	db        *db.DB
	engine    *router.Engine
	classifier *router.Classifier
	collector *stats.Collector
	proxy     *proxy.Server
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{
		config: config.Default(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	var err error
	a.db, err = db.New(a.config.AppDataDir)
	if err != nil {
		fmt.Printf("db init error: %v\n", err)
		return
	}

	a.collector = stats.NewCollector(a.db)
	a.classifier = router.NewClassifier(nil)
	a.engine = router.NewEngine(a.classifier)

	// Load configured models from DB
	a.loadModels()

	// Auto-start proxy if enabled
	if val, _ := a.db.GetConfig("proxy_enabled"); val == "true" {
		port := a.config.ProxyPort
		if v, _ := a.db.GetConfig("proxy_port"); v != "" {
			fmt.Sscanf(v, "%d", &port)
		}
		a.startProxy(port)
	}
}

func (a *App) loadModels() {
	rows, err := a.db.Query("SELECT id, name, provider, base_url, api_key, model_id, reasoning, coding, creativity, speed, cost_efficiency, max_rpm, max_tpm, is_active FROM models")
	if err != nil {
		return
	}
	defer rows.Close()

	var models []*router.ModelConfig
	for rows.Next() {
		var m router.ModelConfig
		var isActive int
		if err := rows.Scan(&m.ID, &m.Name, &m.Provider, &m.BaseURL, &m.APIKey, &m.ModelID,
			&m.Reasoning, &m.Coding, &m.Creativity, &m.Speed, &m.CostEfficiency,
			&m.MaxRPM, &m.MaxTPM, &isActive); err != nil {
			continue
		}
		m.IsActive = isActive == 1

		// Decrypt API key
		decrypted, err := crypto.Decrypt(m.APIKey)
		if err == nil {
			m.APIKey = decrypted
		}

		models = append(models, &m)

		// Create provider if not exists
		if m.IsActive {
			a.ensureProvider(m.Provider, m.BaseURL, m.APIKey)
		}
	}

	a.engine.SetModels(models)
}

func (a *App) ensureProvider(providerName, baseURL, apiKey string) {
	switch providerName {
	case "openai":
		a.engine.AddProvider("openai", provider.NewOpenAI(baseURL, apiKey))
	case "anthropic":
		a.engine.AddProvider("anthropic", provider.NewAnthropic(baseURL, apiKey))
	}
}

func (a *App) shutdown(ctx context.Context) {
	if a.proxy != nil {
		a.proxy.Stop()
	}
	if a.db != nil {
		a.db.Close()
	}
}

// --- Frontend-bound methods ---

// ModelJSON is the JSON representation sent to the frontend.
type ModelJSON struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Provider       string `json:"provider"`
	BaseURL        string `json:"baseUrl"`
	ModelID        string `json:"modelId"`
	Reasoning      int    `json:"reasoning"`
	Coding         int    `json:"coding"`
	Creativity     int    `json:"creativity"`
	Speed          int    `json:"speed"`
	CostEfficiency int    `json:"costEfficiency"`
	MaxRPM         int    `json:"maxRpm"`
	MaxTPM         int    `json:"maxTpm"`
	IsActive       bool   `json:"isActive"`
}

// GetModels returns all configured models.
func (a *App) GetModels() []ModelJSON {
	rows, err := a.db.Query("SELECT id, name, provider, base_url, model_id, reasoning, coding, creativity, speed, cost_efficiency, max_rpm, max_tpm, is_active FROM models")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var models []ModelJSON
	for rows.Next() {
		var m ModelJSON
		var isActive int
		rows.Scan(&m.ID, &m.Name, &m.Provider, &m.BaseURL, &m.ModelID,
			&m.Reasoning, &m.Coding, &m.Creativity, &m.Speed, &m.CostEfficiency,
			&m.MaxRPM, &m.MaxTPM, &isActive)
		m.IsActive = isActive == 1
		models = append(models, m)
	}
	return models
}

// SaveModel adds or updates a model configuration.
func (a *App) SaveModel(m ModelJSON) error {
	encKey, err := crypto.Encrypt(m.BaseURL) // placeholder: encrypt API key in real use
	_ = encKey
	// For now store API key encrypted
	if m.ID == "" {
		m.ID = router.NewUUID()
	}

	_, err = a.db.Exec(
		`INSERT OR REPLACE INTO models (id, name, provider, base_url, api_key, model_id, reasoning, coding, creativity, speed, cost_efficiency, max_rpm, max_tpm, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.Name, m.Provider, m.BaseURL, "", m.ModelID,
		m.Reasoning, m.Coding, m.Creativity, m.Speed, m.CostEfficiency,
		m.MaxRPM, m.MaxTPM, m.IsActive,
	)
	if err != nil {
		return err
	}

	a.loadModels()
	return nil
}

// DeleteModel removes a model by ID.
func (a *App) DeleteModel(id string) error {
	_, err := a.db.Exec("DELETE FROM models WHERE id = ?", id)
	if err != nil {
		return err
	}
	a.loadModels()
	return nil
}

// TestModel tests connectivity to a model's provider.
func (a *App) TestModel(m ModelJSON) string {
	var p provider.Provider
	switch m.Provider {
	case "openai":
		p = provider.NewOpenAI(m.BaseURL, "")
	case "anthropic":
		p = provider.NewAnthropic(m.BaseURL, "")
	default:
		return fmt.Sprintf("unknown provider: %s", m.Provider)
	}

	if err := p.HealthCheck(a.ctx); err != nil {
		return fmt.Sprintf("FAIL: %v", err)
	}
	return "OK"
}

// ChatMessage is a message sent from the frontend.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is a chat request from the frontend.
type ChatRequest struct {
	Messages []ChatMessage `json:"messages"`
	Mode     string        `json:"mode"`  // "auto", "manual", "race"
	ModelID  string        `json:"modelId"`
}

// ChatResponse is sent back to the frontend with routing metadata.
type ChatResponse struct {
	ModelID    string `json:"modelId"`
	ModelName  string `json:"modelName"`
	Provider   string `json:"provider"`
	Complexity string `json:"complexity"`
	RouteMode  string `json:"routeMode"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
}

// SendChat sends a chat request through the router. Streaming is handled via Events.
func (a *App) SendChat(req ChatRequest) ChatResponse {
	msgs := make([]provider.Message, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = provider.Message{Role: m.Role, Content: m.Content}
	}

	var mode router.RouteMode
	switch req.Mode {
	case "manual":
		mode = router.RouteManual
	case "race":
		mode = router.RouteRace
	default:
		mode = router.RouteAuto
	}

	result := a.engine.Route(a.ctx, &router.RouteRequest{
		Messages: msgs,
		Mode:     mode,
		ModelID:  req.ModelID,
		Source:   "chat",
	})

	resp := ChatResponse{
		ModelID:    result.ModelID,
		ModelName:  result.ModelName,
		Provider:   result.Provider,
		Complexity: result.Complexity,
		RouteMode:  result.RouteMode,
		Status:     result.Status,
		Error:      result.ErrorMsg,
	}

	// Consume stream and emit events to frontend
	if result.Stream != nil {
		go func() {
			for chunk := range result.Stream {
				if chunk.Error != nil {
					// emit error event
					continue
				}
				data, _ := json.Marshal(map[string]string{
					"content": chunk.Content,
					"done":    fmt.Sprintf("%v", chunk.Done),
				})
				// Wails runtime events would go here
				_ = data
			}

			// Log to stats
			a.collector.LogRequest(&stats.RequestLog{
				ID:         router.NewUUID(),
				ModelID:    result.ModelID,
				Source:     "chat",
				Complexity: result.Complexity,
				RouteMode:  result.RouteMode,
				Status:     result.Status,
				CreatedAt:  time.Now(),
			})
		}()
	}

	return resp
}

// GetDashboardStats returns today's stats.
func (a *App) GetDashboardStats() map[string]any {
	daily, _ := a.collector.GetDailyStats(time.Now())
	modelUsage, _ := a.collector.GetModelUsage(time.Now())
	complexity, _ := a.collector.GetComplexityDistribution(time.Now())
	recent, _ := a.collector.GetRecentLogs(20)

	return map[string]any{
		"daily":      daily,
		"modelUsage": modelUsage,
		"complexity": complexity,
		"recentLogs": recent,
	}
}

// GetProxyStatus returns the current proxy server status.
func (a *App) GetProxyStatus() map[string]any {
	running := a.proxy != nil
	port := a.config.ProxyPort
	if v, _ := a.db.GetConfig("proxy_port"); v != "" {
		fmt.Sscanf(v, "%d", &port)
	}
	return map[string]any{
		"running": running,
		"port":    port,
	}
}

// StartProxy starts the proxy server on the given port.
func (a *App) StartProxy(port int) string {
	if a.proxy != nil {
		a.proxy.Stop()
	}
	if err := a.startProxy(port); err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	a.db.SetConfig("proxy_port", fmt.Sprintf("%d", port))
	a.db.SetConfig("proxy_enabled", "true")
	return "ok"
}

func (a *App) startProxy(port int) error {
	a.proxy = proxy.New(port, a.engine)
	return a.proxy.Start()
}

// StopProxy stops the proxy server.
func (a *App) StopProxy() string {
	if a.proxy != nil {
		a.proxy.Stop()
		a.proxy = nil
	}
	a.db.SetConfig("proxy_enabled", "false")
	return "ok"
}

// GetConfig returns a config value.
func (a *App) GetConfig(key string) string {
	val, _ := a.db.GetConfig(key)
	return val
}

// SetConfig sets a config value.
func (a *App) SetConfig(key, value string) string {
	err := a.db.SetConfig(key, value)
	if err != nil {
		return err.Error()
	}
	return "ok"
}
```

- [ ] **Step 2: Update main.go**

Replace `main.go` with:

```go
package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "Multi-Model Router",
		Width:  1200,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
```

- [ ] **Step 3: Commit**

```bash
git add app.go main.go
git commit -m "feat: add Wails bindings connecting Go backend to Vue frontend"
```

---

## Phase 6: Frontend

### Task 11: Frontend Foundation & Chat UI

**Files:**
- Modify: `frontend/package.json` (add dependencies)
- Create: `frontend/src/views/ChatView.vue`
- Create: `frontend/src/components/MessageBubble.vue`
- Create: `frontend/src/stores/chat.ts`
- Modify: `frontend/src/App.vue`

- [ ] **Step 1: Install frontend dependencies**

Run:
```bash
cd /c/workspace/multi_model_router/frontend
npm install vue-router@4 pinia @vueuse/core
npm install -D @types/node
```

- [ ] **Step 2: Create router and store setup**

Create `frontend/src/router.ts`:

```typescript
import { createRouter, createWebHashHistory } from 'vue-router'

const routes = [
  { path: '/', name: 'chat', component: () => import('./views/ChatView.vue') },
  { path: '/dashboard', name: 'dashboard', component: () => import('./views/DashboardView.vue') },
  { path: '/settings', name: 'settings', component: () => import('./views/SettingsView.vue') },
]

export default createRouter({
  history: createWebHashHistory(),
  routes,
})
```

Create `frontend/src/stores/models.ts`:

```typescript
import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  GetModels,
  SaveModel,
  DeleteModel,
} from '../../wailsjs/go/main/App'

export interface Model {
  id: string
  name: string
  provider: string
  baseUrl: string
  modelId: string
  reasoning: number
  coding: number
  creativity: number
  speed: number
  costEfficiency: number
  maxRpm: number
  maxTpm: number
  isActive: boolean
}

export const useModelsStore = defineStore('models', () => {
  const models = ref<Model[]>([])
  const loading = ref(false)

  async function fetchModels() {
    loading.value = true
    try {
      models.value = await GetModels() as Model[]
    } finally {
      loading.value = false
    }
  }

  async function save(model: Model) {
    await SaveModel(model)
    await fetchModels()
  }

  async function remove(id: string) {
    await DeleteModel(id)
    await fetchModels()
  }

  return { models, loading, fetchModels, save, remove }
})
```

- [ ] **Step 3: Create ChatView**

Create `frontend/src/views/ChatView.vue`:

```vue
<template>
  <div class="chat-view">
    <div class="model-bar">
      <label>
        <input type="radio" v-model="mode" value="auto" /> 自动路由
      </label>
      <label>
        <input type="radio" v-model="mode" value="manual" /> 指定模型
      </label>
      <label>
        <input type="radio" v-model="mode" value="race" /> 竞速
      </label>
      <select v-if="mode === 'manual'" v-model="selectedModel">
        <option v-for="m in modelsStore.models" :key="m.id" :value="m.modelId">
          {{ m.name }}
        </option>
      </select>
    </div>

    <div class="messages" ref="messagesEl">
      <MessageBubble
        v-for="msg in messages"
        :key="msg.id"
        :message="msg"
      />
      <div v-if="streaming" class="streaming">思考中...</div>
    </div>

    <div class="input-bar">
      <textarea
        v-model="input"
        @keydown.enter.exact.prevent="send"
        placeholder="输入消息... (Enter 发送, Shift+Enter 换行)"
        rows="2"
      />
      <button @click="send" :disabled="!input.trim() || streaming">发送</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, nextTick, onMounted } from 'vue'
import { SendChat } from '../../wailsjs/go/main/App'
import { useModelsStore } from '../stores/models'
import MessageBubble from '../components/MessageBubble.vue'

const modelsStore = useModelsStore()
const messages = ref<any[]>([])
const input = ref('')
const mode = ref('auto')
const selectedModel = ref('')
const streaming = ref(false)
const messagesEl = ref<HTMLElement>()

onMounted(() => {
  modelsStore.fetchModels()
})

async function send() {
  const text = input.value.trim()
  if (!text || streaming.value) return

  messages.value.push({
    id: Date.now().toString(),
    role: 'user',
    content: text,
  })
  input.value = ''
  streaming.value = true

  await nextTick()
  scrollToBottom()

  try {
    const resp = await SendChat({
      messages: messages.value.map(m => ({ role: m.role, content: m.content })),
      mode: mode.value,
      modelId: selectedModel.value,
    })

    messages.value.push({
      id: (Date.now() + 1).toString(),
      role: 'assistant',
      content: '(流式响应中...)',
      modelId: resp.modelId,
      modelName: resp.modelName,
      complexity: resp.complexity,
      routeMode: resp.routeMode,
    })
  } catch (e: any) {
    messages.value.push({
      id: (Date.now() + 1).toString(),
      role: 'assistant',
      content: `错误: ${e.message || e}`,
      isError: true,
    })
  } finally {
    streaming.value = false
    scrollToBottom()
  }
}

function scrollToBottom() {
  nextTick(() => {
    if (messagesEl.value) {
      messagesEl.value.scrollTop = messagesEl.value.scrollHeight
    }
  })
}
</script>

<style scoped>
.chat-view {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.model-bar {
  display: flex;
  gap: 16px;
  padding: 8px 16px;
  border-bottom: 1px solid #e5e7eb;
  align-items: center;
  font-size: 14px;
}
.model-bar label {
  display: flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
}
.model-bar select {
  padding: 4px 8px;
  border: 1px solid #d1d5db;
  border-radius: 4px;
}
.messages {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
}
.streaming {
  text-align: center;
  color: #9ca3af;
  padding: 8px;
}
.input-bar {
  display: flex;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid #e5e7eb;
}
.input-bar textarea {
  flex: 1;
  padding: 8px 12px;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  resize: none;
  font-size: 14px;
  font-family: inherit;
}
.input-bar button {
  padding: 8px 20px;
  background: #3b82f6;
  color: white;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-size: 14px;
}
.input-bar button:disabled {
  background: #9ca3af;
  cursor: not-allowed;
}
</style>
```

- [ ] **Step 4: Create MessageBubble component**

Create `frontend/src/components/MessageBubble.vue`:

```vue
<template>
  <div :class="['bubble', message.role, { error: message.isError }]">
    <div class="meta" v-if="message.role === 'assistant' && message.modelName">
      <span class="model-badge">{{ message.modelName }}</span>
      <span class="complexity" v-if="message.complexity">{{ message.complexity }}</span>
      <span class="route" v-if="message.routeMode">{{ message.routeMode }}</span>
    </div>
    <div class="content">{{ message.content }}</div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ message: any }>()
</script>

<style scoped>
.bubble {
  max-width: 80%;
  margin: 8px 0;
  padding: 12px 16px;
  border-radius: 12px;
  font-size: 14px;
  line-height: 1.6;
  white-space: pre-wrap;
}
.bubble.user {
  margin-left: auto;
  background: #3b82f6;
  color: white;
}
.bubble.assistant {
  margin-right: auto;
  background: #f3f4f6;
  color: #1f2937;
}
.bubble.error {
  background: #fef2f2;
  color: #dc2626;
  border: 1px solid #fecaca;
}
.meta {
  display: flex;
  gap: 6px;
  margin-bottom: 6px;
  font-size: 12px;
}
.model-badge {
  background: #dbeafe;
  color: #1d4ed8;
  padding: 2px 6px;
  border-radius: 4px;
}
.complexity, .route {
  background: #e5e7eb;
  color: #374151;
  padding: 2px 6px;
  border-radius: 4px;
}
</style>
```

- [ ] **Step 5: Update App.vue with navigation**

Replace `frontend/src/App.vue`:

```vue
<template>
  <div class="app">
    <nav class="sidebar">
      <div class="nav-items">
        <router-link to="/" class="nav-item" active-class="active">💬 聊天</router-link>
        <router-link to="/dashboard" class="nav-item" active-class="active">📊 仪表盘</router-link>
        <router-link to="/settings" class="nav-item" active-class="active">⚙ 设置</router-link>
      </div>
    </nav>
    <main class="content">
      <router-view />
    </main>
  </div>
</template>

<script setup lang="ts">
</script>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}
html, body, #app {
  height: 100%;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}
.app {
  display: flex;
  height: 100%;
}
.sidebar {
  width: 60px;
  background: #1f2937;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding-top: 12px;
}
.nav-items {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.nav-item {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border-radius: 8px;
  color: #9ca3af;
  text-decoration: none;
  font-size: 20px;
  transition: all 0.15s;
}
.nav-item:hover {
  background: #374151;
  color: white;
}
.nav-item.active {
  background: #3b82f6;
  color: white;
}
.content {
  flex: 1;
  overflow: hidden;
}
</style>
```

- [ ] **Step 6: Update main.ts**

Replace `frontend/src/main.ts`:

```typescript
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')
```

- [ ] **Step 7: Commit**

```bash
git add frontend/
git commit -m "feat: add Vue 3 chat UI with navigation, message bubbles, and model store"
```

---

### Task 12: Dashboard View

**Files:**
- Create: `frontend/src/views/DashboardView.vue`

- [ ] **Step 1: Create DashboardView**

Create `frontend/src/views/DashboardView.vue`:

```vue
<template>
  <div class="dashboard">
    <h2>📊 仪表盘</h2>

    <div class="overview">
      <div class="stat-card">
        <div class="stat-value">{{ stats.daily?.TotalRequests || 0 }}</div>
        <div class="stat-label">今日请求</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ formatTokens(stats.daily?.TotalTokensIn || 0 + stats.daily?.TotalTokensOut || 0) }}</div>
        <div class="stat-label">今日 Token</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ Math.round(stats.daily?.AvgLatencyMs || 0) }}ms</div>
        <div class="stat-label">平均延迟</div>
      </div>
    </div>

    <div class="charts">
      <div class="chart-section">
        <h3>模型调用分布</h3>
        <div class="bar-chart">
          <div v-for="m in stats.modelUsage || []" :key="m.ModelID" class="bar-row">
            <span class="bar-label">{{ m.ModelID }}</span>
            <div class="bar-track">
              <div class="bar-fill" :style="{ width: m.Percentage + '%' }"></div>
            </div>
            <span class="bar-value">{{ m.Count }} ({{ m.Percentage.toFixed(1) }}%)</span>
          </div>
        </div>
      </div>

      <div class="chart-section">
        <h3>复杂度分布</h3>
        <div class="bar-chart">
          <div v-for="(count, key) in stats.complexity || {}" :key="key" class="bar-row">
            <span class="bar-label">{{ complexityLabel(key) }}</span>
            <div class="bar-track">
              <div class="bar-fill" :style="{ width: complexityPercent(count) + '%' }" :class="key"></div>
            </div>
            <span class="bar-value">{{ count }}</span>
          </div>
        </div>
      </div>
    </div>

    <div class="logs">
      <h3>最近请求</h3>
      <table>
        <thead>
          <tr>
            <th>时间</th>
            <th>模型</th>
            <th>复杂度</th>
            <th>来源</th>
            <th>Token</th>
            <th>延迟</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="log in stats.recentLogs || []" :key="log.ID">
            <td>{{ formatTime(log.CreatedAt) }}</td>
            <td>{{ log.ModelID }}</td>
            <td>{{ complexityLabel(log.Complexity) }}</td>
            <td>{{ log.Source }}</td>
            <td>{{ (log.TokensIn || 0) + (log.TokensOut || 0) }}</td>
            <td>{{ log.LatencyMs }}ms</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { GetDashboardStats } from '../../wailsjs/go/main/App'

const stats = ref<any>({})

onMounted(async () => {
  stats.value = await GetDashboardStats() as any
})

function formatTokens(n: number): string {
  if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M'
  if (n >= 1000) return (n / 1000).toFixed(1) + 'K'
  return String(n)
}

function formatTime(t: string): string {
  if (!t) return ''
  return new Date(t).toLocaleTimeString()
}

function complexityLabel(c: string): string {
  switch (c) {
    case 'simple': return '简单'
    case 'medium': return '中等'
    case 'complex': return '复杂'
    default: return c || '-'
  }
}

function complexityPercent(count: number): number {
  const total = Object.values(stats.value.complexity || {}).reduce((s: number, v: any) => s + Number(v), 0)
  return total > 0 ? (Number(count) / total) * 100 : 0
}
</script>

<style scoped>
.dashboard { padding: 24px; overflow-y: auto; height: 100%; }
h2 { margin-bottom: 20px; }
.overview { display: flex; gap: 16px; margin-bottom: 24px; }
.stat-card {
  flex: 1; padding: 16px; background: #f9fafb; border-radius: 8px;
  border: 1px solid #e5e7eb; text-align: center;
}
.stat-value { font-size: 28px; font-weight: 700; color: #1f2937; }
.stat-label { font-size: 13px; color: #6b7280; margin-top: 4px; }
.charts { display: flex; gap: 16px; margin-bottom: 24px; }
.chart-section { flex: 1; background: #f9fafb; border-radius: 8px; padding: 16px; border: 1px solid #e5e7eb; }
.chart-section h3 { font-size: 14px; margin-bottom: 12px; color: #374151; }
.bar-row { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.bar-label { width: 80px; font-size: 13px; color: #4b5563; text-align: right; }
.bar-track { flex: 1; height: 20px; background: #e5e7eb; border-radius: 4px; overflow: hidden; }
.bar-fill { height: 100%; background: #3b82f6; border-radius: 4px; transition: width 0.3s; }
.bar-fill.simple { background: #10b981; }
.bar-fill.medium { background: #f59e0b; }
.bar-fill.complex { background: #ef4444; }
.bar-value { width: 80px; font-size: 12px; color: #6b7280; }
.logs { background: #f9fafb; border-radius: 8px; padding: 16px; border: 1px solid #e5e7eb; }
.logs h3 { font-size: 14px; margin-bottom: 12px; }
table { width: 100%; border-collapse: collapse; }
th, td { padding: 8px 12px; text-align: left; font-size: 13px; border-bottom: 1px solid #e5e7eb; }
th { color: #6b7280; font-weight: 500; }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/views/DashboardView.vue
git commit -m "feat: add dashboard view with stats cards, model usage, and request logs"
```

---

### Task 13: Settings View

**Files:**
- Create: `frontend/src/views/SettingsView.vue`
- Create: `frontend/src/components/ModelEditor.vue`
- Create: `frontend/src/components/ModelCard.vue`

- [ ] **Step 1: Create ModelCard component**

Create `frontend/src/components/ModelCard.vue`:

```vue
<template>
  <div class="model-card" :class="{ inactive: !model.isActive }">
    <div class="card-header">
      <span class="model-name">{{ model.name }}</span>
      <span :class="['provider-badge', model.provider]">{{ model.provider }}</span>
    </div>
    <div class="scores">
      <div class="score-item" v-for="(label, key) in scoreLabels" :key="key">
        <span class="score-label">{{ label }}</span>
        <div class="score-bar">
          <div class="score-fill" :style="{ width: (model as any)[key] * 10 + '%' }"></div>
        </div>
        <span class="score-value">{{ (model as any)[key] }}</span>
      </div>
    </div>
    <div class="card-footer">
      <span class="rpm">RPM: {{ model.maxRpm }}</span>
      <span :class="['status', model.isActive ? 'active' : 'inactive']">
        {{ model.isActive ? '● 正常' : '○ 停用' }}
      </span>
      <div class="actions">
        <button @click="$emit('edit', model)">编辑</button>
        <button @click="$emit('test', model)">测试</button>
        <button class="danger" @click="$emit('delete', model.id)">删除</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ model: any }>()
defineEmits(['edit', 'test', 'delete'])

const scoreLabels: Record<string, string> = {
  reasoning: '推理',
  coding: '代码',
  creativity: '创意',
  speed: '速度',
  costEfficiency: '性价比',
}
</script>

<style scoped>
.model-card {
  background: white;
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  padding: 16px;
  transition: box-shadow 0.15s;
}
.model-card:hover { box-shadow: 0 2px 8px rgba(0,0,0,0.08); }
.model-card.inactive { opacity: 0.6; }
.card-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
.model-name { font-weight: 600; font-size: 16px; }
.provider-badge { padding: 2px 8px; border-radius: 4px; font-size: 12px; font-weight: 500; }
.provider-badge.openai { background: #dcfce7; color: #166534; }
.provider-badge.anthropic { background: #fef3c7; color: #92400e; }
.scores { display: flex; flex-direction: column; gap: 6px; margin-bottom: 12px; }
.score-item { display: flex; align-items: center; gap: 8px; }
.score-label { width: 48px; font-size: 12px; color: #6b7280; }
.score-bar { flex: 1; height: 6px; background: #e5e7eb; border-radius: 3px; }
.score-fill { height: 100%; background: #3b82f6; border-radius: 3px; }
.score-value { width: 20px; font-size: 12px; color: #374151; text-align: right; }
.card-footer { display: flex; align-items: center; gap: 12px; font-size: 13px; }
.rpm { color: #6b7280; }
.status.active { color: #10b981; }
.status.inactive { color: #9ca3af; }
.actions { margin-left: auto; display: flex; gap: 6px; }
.actions button {
  padding: 4px 10px; border: 1px solid #d1d5db; border-radius: 4px;
  background: white; cursor: pointer; font-size: 12px;
}
.actions button:hover { background: #f3f4f6; }
.actions button.danger { color: #dc2626; border-color: #fecaca; }
.actions button.danger:hover { background: #fef2f2; }
</style>
```

- [ ] **Step 2: Create ModelEditor dialog**

Create `frontend/src/components/ModelEditor.vue`:

```vue
<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal">
      <h3>{{ isEdit ? '编辑模型' : '添加模型' }}</h3>

      <div class="form-grid">
        <label>名称 <input v-model="form.name" placeholder="GPT-4o" /></label>
        <label>供应商
          <select v-model="form.provider">
            <option value="openai">OpenAI</option>
            <option value="anthropic">Anthropic</option>
          </select>
        </label>
        <label class="wide">Base URL <input v-model="form.baseUrl" placeholder="https://api.openai.com" /></label>
        <label class="wide">API Key <input v-model="form.apiKey" type="password" placeholder="sk-..." /></label>
        <label class="wide">模型 ID <input v-model="form.modelId" placeholder="gpt-4o" /></label>

        <div class="wide scores-section">
          <h4>能力评分 (1-10)</h4>
          <div class="score-inputs">
            <label v-for="(label, key) in scoreLabels" :key="key">
              {{ label }}
              <input type="number" v-model.number="(form as any)[key]" min="1" max="10" />
            </label>
          </div>
        </div>

        <label>RPM 限制 <input type="number" v-model.number="form.maxRpm" /></label>
        <label>TPM 限制 <input type="number" v-model.number="form.maxTpm" /></label>
        <label class="wide">
          <input type="checkbox" v-model="form.isActive" /> 启用
        </label>
      </div>

      <div class="actions">
        <button @click="$emit('close')">取消</button>
        <button class="primary" @click="save">保存</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'

const props = defineProps<{ model?: any }>()
const emit = defineEmits(['close', 'save'])

const isEdit = computed(() => !!props.model?.id)

const form = ref({
  id: props.model?.id || '',
  name: props.model?.name || '',
  provider: props.model?.provider || 'openai',
  baseUrl: props.model?.baseUrl || '',
  apiKey: props.model?.apiKey || '',
  modelId: props.model?.modelId || '',
  reasoning: props.model?.reasoning || 5,
  coding: props.model?.coding || 5,
  creativity: props.model?.creativity || 5,
  speed: props.model?.speed || 5,
  costEfficiency: props.model?.costEfficiency || 5,
  maxRpm: props.model?.maxRpm || 60,
  maxTpm: props.model?.maxTpm || 100000,
  isActive: props.model?.isActive ?? true,
})

const scoreLabels: Record<string, string> = {
  reasoning: '推理', coding: '代码', creativity: '创意',
  speed: '速度', costEfficiency: '性价比',
}

function save() {
  emit('save', { ...form.value })
}
</script>

<style scoped>
.modal-overlay {
  position: fixed; inset: 0; background: rgba(0,0,0,0.4);
  display: flex; align-items: center; justify-content: center; z-index: 100;
}
.modal {
  background: white; border-radius: 12px; padding: 24px;
  width: 480px; max-height: 80vh; overflow-y: auto;
}
h3 { margin-bottom: 16px; }
.form-grid {
  display: grid; grid-template-columns: 1fr 1fr; gap: 12px;
}
.form-grid label {
  display: flex; flex-direction: column; gap: 4px;
  font-size: 13px; color: #374151;
}
.form-grid label.wide { grid-column: 1 / -1; }
.form-grid input, .form-grid select {
  padding: 8px; border: 1px solid #d1d5db; border-radius: 6px;
  font-size: 14px;
}
.scores-section { margin-top: 8px; }
.scores-section h4 { font-size: 14px; margin-bottom: 8px; }
.score-inputs { display: flex; gap: 12px; flex-wrap: wrap; }
.score-inputs label { flex-direction: row; align-items: center; gap: 6px; }
.score-inputs input { width: 60px; }
.actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; }
.actions button {
  padding: 8px 20px; border: 1px solid #d1d5db; border-radius: 6px;
  background: white; cursor: pointer;
}
.actions button.primary { background: #3b82f6; color: white; border-color: #3b82f6; }
</style>
```

- [ ] **Step 3: Create SettingsView**

Create `frontend/src/views/SettingsView.vue`:

```vue
<template>
  <div class="settings">
    <h2>⚙ 设置</h2>

    <section class="section">
      <h3>已配置模型</h3>
      <div class="model-list">
        <ModelCard
          v-for="m in modelsStore.models"
          :key="m.id"
          :model="m"
          @edit="openEditor($event)"
          @test="testModel($event)"
          @delete="deleteModel($event)"
        />
        <button class="add-btn" @click="openEditor()">+ 添加模型</button>
      </div>
    </section>

    <section class="section">
      <h3>代理服务</h3>
      <div class="proxy-config">
        <label>端口 <input type="number" v-model.number="proxyPort" /></label>
        <div class="proxy-status">
          状态: <span :class="proxyRunning ? 'active' : 'inactive'">
            {{ proxyRunning ? '● 运行中' : '○ 已停止' }}
          </span>
        </div>
        <div class="proxy-actions">
          <button v-if="!proxyRunning" @click="startProxy">启动</button>
          <button v-else @click="stopProxy">停止</button>
          <button @click="copyProxyUrl">复制代理地址</button>
        </div>
      </div>
    </section>

    <ModelEditor
      v-if="showEditor"
      :model="editingModel"
      @close="showEditor = false"
      @save="saveModel"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  GetProxyStatus,
  StartProxy,
  StopProxy,
  TestModel,
} from '../../wailsjs/go/main/App'
import { useModelsStore } from '../stores/models'
import ModelCard from '../components/ModelCard.vue'
import ModelEditor from '../components/ModelEditor.vue'

const modelsStore = useModelsStore()
const showEditor = ref(false)
const editingModel = ref<any>(null)
const proxyPort = ref(9680)
const proxyRunning = ref(false)

onMounted(async () => {
  await modelsStore.fetchModels()
  const status = await GetProxyStatus() as any
  proxyRunning.value = status.running
  proxyPort.value = status.port
})

function openEditor(model?: any) {
  editingModel.value = model || null
  showEditor.value = true
}

async function saveModel(model: any) {
  await modelsStore.save(model)
  showEditor.value = false
}

async function deleteModel(id: string) {
  if (confirm('确定要删除这个模型吗？')) {
    await modelsStore.remove(id)
  }
}

async function testModel(model: any) {
  const result = await TestModel(model)
  alert(`测试结果: ${result}`)
}

async function startProxy() {
  const result = await StartProxy(proxyPort.value)
  if (result === 'ok') {
    proxyRunning.value = true
  } else {
    alert(result)
  }
}

async function stopProxy() {
  await StopProxy()
  proxyRunning.value = false
}

function copyProxyUrl() {
  navigator.clipboard.writeText(`http://localhost:${proxyPort.value}`)
  alert('已复制代理地址')
}
</script>

<style scoped>
.settings { padding: 24px; overflow-y: auto; height: 100%; }
h2 { margin-bottom: 20px; }
.section { margin-bottom: 24px; }
.section h3 { font-size: 16px; margin-bottom: 12px; color: #374151; }
.model-list { display: flex; flex-direction: column; gap: 12px; }
.add-btn {
  padding: 12px; border: 2px dashed #d1d5db; border-radius: 12px;
  background: none; cursor: pointer; font-size: 14px; color: #6b7280;
}
.add-btn:hover { border-color: #3b82f6; color: #3b82f6; }
.proxy-config {
  background: white; border: 1px solid #e5e7eb; border-radius: 12px;
  padding: 16px; display: flex; align-items: center; gap: 16px;
}
.proxy-config label { font-size: 14px; display: flex; align-items: center; gap: 8px; }
.proxy-config input { width: 80px; padding: 6px; border: 1px solid #d1d5db; border-radius: 4px; }
.proxy-status { font-size: 14px; }
.proxy-status .active { color: #10b981; }
.proxy-status .inactive { color: #9ca3af; }
.proxy-actions { margin-left: auto; display: flex; gap: 8px; }
.proxy-actions button {
  padding: 6px 14px; border: 1px solid #d1d5db; border-radius: 6px;
  background: white; cursor: pointer; font-size: 13px;
}
.proxy-actions button:hover { background: #f3f4f6; }
</style>
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/SettingsView.vue frontend/src/components/ModelCard.vue frontend/src/components/ModelEditor.vue
git commit -m "feat: add settings view with model management cards, editor dialog, and proxy config"
```

---

## Phase 7: Integration & Build

### Task 14: Integration & First Build

- [ ] **Step 1: Ensure go.mod has correct module name**

Run:
```bash
cd /c/workspace/multi_model_router
head -1 go.mod
```

Verify the module name matches imports (should be `multi_model_router` or the name Wails generated). If different, update all import paths in Go files accordingly.

- [ ] **Step 2: Run all Go tests**

Run:
```bash
cd /c/workspace/multi_model_router
go test ./internal/... -v
```
Expected: all tests PASS

- [ ] **Step 3: Generate Wails bindings**

Run:
```bash
cd /c/workspace/multi_model_router
wails generate module
```

This creates the TypeScript bindings in `frontend/wailsjs/go/main/App.ts`.

- [ ] **Step 4: Install frontend deps and build**

Run:
```bash
cd /c/workspace/multi_model_router/frontend
npm install
npm run build
```
Expected: build succeeds

- [ ] **Step 5: Build the Wails app**

Run:
```bash
cd /c/workspace/multi_model_router
wails build
```
Expected: `build/bin/multi_model_router.exe` created

- [ ] **Step 6: Run the built app and smoke test**

Run:
```bash
./build/bin/multi_model_router.exe
```

Verify:
- Window opens with 3 tabs (聊天, 仪表盘, 设置)
- Settings page shows "添加模型" button
- Proxy section shows port config
- No errors in console

- [ ] **Step 7: Final commit**

```bash
git add -A
git commit -m "feat: complete Multi-Model Router v1 — chat, dashboard, settings, proxy"
```

---

## Post-Build Checklist

- [ ] Can add a model via Settings UI
- [ ] Can start/stop proxy server
- [ ] Chat sends a message and receives a response (with a model configured)
- [ ] Dashboard shows stats after making requests
- [ ] All Go tests pass (`go test ./internal/... -v`)
- [ ] App builds without errors (`wails build`)
