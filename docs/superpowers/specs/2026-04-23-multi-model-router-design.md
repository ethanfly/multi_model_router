# Multi-Model Router Desktop App — Design Spec

## Overview

A desktop application (Wails + Vue 3 + Go) that acts as a local LLM gateway with intelligent routing. It supports automatic model selection based on question complexity, provides a built-in chat interface, and exposes a local proxy for external tools.

## Architecture

### Approach: Hybrid Mode (Proxy Gateway + Built-in Chat + Config Dashboard)

```
┌─────────────────────────────────────────────────────────┐
│                   Wails Desktop App                      │
│                                                          │
│  ┌─────────────────────────────────────────────────────┐ │
│  │              Vue 3 Frontend (Webview)               │ │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────────────────┐ │ │
│  │  │  Chat UI  │ │Dashboard │ │  Settings / Models   │ │ │
│  │  └──────────┘ └──────────┘ └──────────────────────┘ │ │
│  └──────────────────────┬──────────────────────────────┘ │
│                         │ Wails Bindings                 │
│  ┌──────────────────────┴──────────────────────────────┐ │
│  │                 Go Backend                            │ │
│  │                                                      │ │
│  │  ┌─────────────┐  ┌──────────┐  ┌────────────────┐  │ │
│  │  │ Router Core │  │  Proxy   │  │  Config Store  │  │ │
│  │  │  (路由引擎)  │  │ (代理层)  │  │  (SQLite持久)  │  │ │
│  │  └─────────────┘  └──────────┘  └────────────────┘  │ │
│  │         │                                          │ │
│  │  ┌──────┴───────┐  ┌────────────────────────────┐   │ │
│  │  │  Classifier  │  │     API Adapters            │   │ │
│  │  │ (问题分类器)  │  │  ┌──────┐  ┌───────────┐  │   │ │
│  │  └──────────────┘  │  │OpenAI│  │ Anthropic  │  │   │ │
│  │                    │  └──────┘  └───────────┘  │   │ │
│  │                    └────────────────────────────┘   │ │
│  └─────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
                          │
                    ┌─────┴─────┐
                    │  外部工具   │  ← localhost:9680 代理入口
                    │ IDE/脚本等  │
                    └───────────┘
```

### Core Flow

1. Request enters (built-in chat or external proxy) → Router Engine
2. Router Engine invokes Classifier to determine question complexity
3. Classifier uses local rules first; if uncertain, triggers small model pre-analysis
4. Based on complexity + model capability scores + rate limits + health status → selects optimal model
5. Sends request via the matching API Adapter; supports streaming responses
6. On failure, automatically falls back to next-best model; logs usage stats

### Data Persistence

SQLite (embedded, single-file, ships with the app). API keys encrypted with AES.

## Router Engine

### Classifier (Hybrid Strategy)

**Layer 1: Local Rule Engine (zero latency)**

Analyzes the question using keyword matching, regex patterns, length/structure heuristics.

| Signal               | Simple                    | Medium                     | Complex                     |
|----------------------|---------------------------|----------------------------|-----------------------------|
| Keywords             | translate, summarize, rewrite | analyze, compare, optimize | design, architect, derive   |
| Length               | <50 chars                 | 50-200 chars               | >200 chars                  |
| Code blocks          | None                      | Simple snippet             | Multi-file / system-level   |
| Math / reasoning     | None                      | Basic arithmetic           | Multi-step reasoning / proof|

**Layer 2: Small Model Pre-Analysis (triggered only when Layer 1 is uncertain)**

- Uses the fastest/cheapest configured model
- Prompt asks for a 1-5 complexity score + category tags
- 2-second timeout; falls back to "medium" on timeout

### Model Capability Scoring

Each model is scored on 5 dimensions (1-10):

```json
{
  "reasoning": 8,
  "coding": 9,
  "creativity": 7,
  "speed": 9,
  "cost_efficiency": 8
}
```

### Routing Decision Flow

```
Incoming request
  │
  ├─ Race mode? → Send to N models simultaneously, return fastest
  │
  └─ Normal routing:
      │
      ├─ 1. Classifier determines complexity (simple/medium/complex)
      ├─ 2. Filter available models (exclude unhealthy + rate-limited)
      ├─ 3. Match by complexity:
      │     simple  → speed + cost_efficiency weighted high
      │     medium  → balanced weights
      │     complex → reasoning + coding weighted high
      ├─ 4. Sort by weighted score, pick highest
      └─ 5. Send request; on failure, retry with next-best model
```

Weight configuration per complexity level:

| Dimension        | Simple | Medium | Complex |
|-----------------|--------|--------|---------|
| reasoning       | 0.1    | 0.2    | 0.35    |
| coding          | 0.1    | 0.2    | 0.3     |
| creativity      | 0.15   | 0.2    | 0.1     |
| speed           | 0.35   | 0.2    | 0.1     |
| cost_efficiency | 0.3    | 0.2    | 0.15    |

## API Adapter Layer

### Unified Provider Interface

```go
type Provider interface {
    ChatCompletion(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)
    ListModels(ctx context.Context) ([]ModelInfo, error)
    HealthCheck(ctx context.Context) error
}
```

### OpenAI Adapter

- Endpoint: `/v1/chat/completions`
- Supports SSE streaming, Function Calling, JSON Mode

### Anthropic Adapter

- Endpoint: `/v1/messages`
- Auto-converts between OpenAI format ↔ Anthropic format (message structure, role mapping)
- Supports Anthropic-specific `thinking` extension

## Local Proxy Server

```
External tool → http://localhost:9680/v1/chat/completions
                                │
                     ┌──────────┴──────────┐
                     │ Auto-detect format   │
                     │ (OpenAI/Anthropic)   │
                     └──────────┬──────────┘
                                │
                     Router → select model → forward
```

- Default port: 9680 (configurable)
- Auto-detects request format via headers (`x-api-key` vs `Authorization: Bearer`)
- Streaming passthrough: SSE chunks forwarded directly, zero-buffer delay
- Router header extension: `X-Router-Mode: speed|quality|race` lets callers override routing strategy

## Frontend UI

### Tab 1: Chat

- Left sidebar: collapsible conversation history list (grouped by date)
- Model selection bar: toggle between auto-routing and manual model selection
- Chat area: message bubbles showing model name, routing decision, capability scores, token usage, latency
- Input area: text input with send button

### Tab 2: Dashboard

- Today overview: total requests, total tokens, average latency
- Model usage distribution (pie/bar chart)
- Complexity distribution (bar chart)
- Recent request log table (time, question summary, model, latency, tokens)

### Tab 3: Settings

- Configured models list: each as a card showing provider, URL, capability scores (radar chart), RPM, health status
- Per-model actions: Edit, Test, Delete
- "Add Model" button
- Proxy service section: port config, start/stop button, copy proxy address

## Data Model (SQLite)

### Tables

```sql
CREATE TABLE models (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    provider        TEXT NOT NULL,        -- 'openai' | 'anthropic'
    base_url        TEXT NOT NULL,
    api_key         TEXT NOT NULL,        -- AES encrypted
    model_id        TEXT NOT NULL,        -- e.g. 'gpt-4o', 'claude-sonnet-4-20250514'
    reasoning       INTEGER DEFAULT 5,
    coding          INTEGER DEFAULT 5,
    creativity      INTEGER DEFAULT 5,
    speed           INTEGER DEFAULT 5,
    cost_efficiency INTEGER DEFAULT 5,
    max_rpm         INTEGER DEFAULT 60,
    max_tpm         INTEGER DEFAULT 100000,
    is_active       BOOLEAN DEFAULT TRUE,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE conversations (
    id          TEXT PRIMARY KEY,
    title       TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE messages (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL,       -- 'user' | 'assistant' | 'system'
    content         TEXT NOT NULL,
    model_id        TEXT,                -- no FK: preserve history after model deletion
    complexity      TEXT,                -- 'simple' | 'medium' | 'complex'
    tokens_in       INTEGER DEFAULT 0,
    tokens_out      INTEGER DEFAULT 0,
    latency_ms      INTEGER DEFAULT 0,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE request_logs (
    id          TEXT PRIMARY KEY,
    model_id    TEXT NOT NULL,
    source      TEXT NOT NULL,           -- 'chat' | 'proxy'
    complexity  TEXT,
    route_mode  TEXT NOT NULL,           -- 'auto' | 'manual' | 'race'
    status      TEXT NOT NULL,           -- 'success' | 'failed' | 'fallback'
    tokens_in   INTEGER DEFAULT 0,
    tokens_out  INTEGER DEFAULT 0,
    latency_ms  INTEGER DEFAULT 0,
    error_msg   TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE app_config (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL
);
```

## Project Structure

```
multi_model_router/
├── build/                      # Wails build config
├── frontend/                   # Vue 3 frontend
│   ├── src/
│   │   ├── views/
│   │   │   ├── ChatView.vue
│   │   │   ├── DashboardView.vue
│   │   │   └── SettingsView.vue
│   │   ├── components/
│   │   │   ├── MessageBubble.vue
│   │   │   ├── ModelCard.vue
│   │   │   ├── StatsChart.vue
│   │   │   └── ModelEditor.vue
│   │   ├── stores/
│   │   │   ├── chat.ts
│   │   │   ├── models.ts
│   │   │   └── config.ts
│   │   ├── App.vue
│   │   └── main.ts
│   ├── index.html
│   ├── package.json
│   └── vite.config.ts
├── internal/                   # Go internal packages
│   ├── router/
│   │   ├── engine.go               # Router core
│   │   └── classifier.go           # Question classifier
│   ├── provider/
│   │   ├── provider.go             # Provider interface
│   │   ├── openai.go               # OpenAI adapter
│   │   └── anthropic.go            # Anthropic adapter
│   ├── proxy/
│   │   └── server.go               # Local proxy server
│   ├── db/
│   │   ├── db.go                   # Database init
│   │   └── migrations/             # SQL migration files
│   ├── stats/
│   │   └── collector.go            # Usage stats collector
│   └── config/
│       └── config.go               # Config management
├── app.go                      # Wails app entry
├── main.go                     # Go main
├── wails.json                  # Wails project config
├── go.mod
└── go.sum
```

## Key Technical Decisions

| Decision                | Choice                    | Rationale                                              |
|-------------------------|---------------------------|--------------------------------------------------------|
| Desktop framework       | Wails v2                  | Go backend for concurrency, small binary, native feel  |
| Frontend framework      | Vue 3 + TypeScript        | Composition API, good Wails support, reactive UI       |
| Database                | SQLite                    | Embedded, zero-config, single-file, ships with app     |
| API key storage         | AES-256 encryption        | Security requirement for local credential storage      |
| Streaming               | SSE passthrough           | Zero-buffer forwarding for real-time chat experience   |
| Rate limiting           | Token bucket per model    | Standard algorithm, easy to implement in Go            |
| Health checking         | Periodic HTTP ping        | Background goroutine checks model endpoints every 30s  |
| ID generation           | UUID v4                   | No coordination needed, safe for distributed scenarios |

## Race Mode Behavior

When race mode is active, the request is sent to N models simultaneously. The **first model to begin streaming a response wins** — its stream is forwarded to the client and all other pending requests are cancelled. This ensures lowest-latency response without wasting tokens on duplicate completions.

## API Key Security

API keys are encrypted at rest using AES-256-GCM. The encryption key is derived from a machine-specific identifier (hostname + hardware fingerprint via SHA-256), requiring no additional user configuration. Keys are decrypted only in memory when making API calls.

## Error Handling

- **Model unavailable**: Auto-fallback to next-best model; log failure in request_logs
- **All models fail**: Return error to user with list of attempted models and failure reasons
- **Rate limit hit**: Queue request briefly (up to 5s), then fallback if still limited
- **Classifier timeout**: Default to "medium" complexity; route with balanced weights
- **Proxy server port conflict**: Auto-increment port (9680 → 9681 → ...) and notify user

## Out of Scope (v1)

- Plugin/extension system
- Multi-user support
- Cloud sync
- Custom prompt templates
- Function calling passthrough
- Image/multimodal input
- Token-level cost estimation (would require per-model pricing data)
