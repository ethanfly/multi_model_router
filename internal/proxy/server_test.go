package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"multi_model_router/internal/router"
)

func TestConvertToolsForUpstream_AnthropicToOpenAI(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"tools": json.RawMessage(`[
			{"name": "get_weather", "description": "Get weather", "input_schema": {"type": "object", "properties": {"city": {"type": "string"}}}}
		]`),
	}

	convertToolsForUpstream(reqMap, true, false)

	var tools []map[string]json.RawMessage
	if err := json.Unmarshal(reqMap["tools"], &tools); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	// Must have "type" and "function" keys
	if string(tools[0]["type"]) != `"function"` {
		t.Errorf("expected type='function', got %s", string(tools[0]["type"]))
	}
	if tools[0]["function"] == nil {
		t.Error("expected 'function' key to be present")
	}

	// Verify function contents
	var fn map[string]json.RawMessage
	if err := json.Unmarshal(tools[0]["function"], &fn); err != nil {
		t.Fatalf("function is not valid JSON: %v", err)
	}
	if string(fn["name"]) != `"get_weather"` {
		t.Errorf("expected name='get_weather', got %s", string(fn["name"]))
	}
	if fn["parameters"] == nil {
		t.Error("expected 'parameters' key (converted from input_schema)")
	}
}

func TestConvertToolsForUpstream_OpenAIToAnthropic(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"tools": json.RawMessage(`[
			{"type": "function", "function": {"name": "get_weather", "description": "Get weather", "parameters": {"type": "object", "properties": {"city": {"type": "string"}}}}}
		]`),
	}

	convertToolsForUpstream(reqMap, false, true)

	var tools []map[string]json.RawMessage
	if err := json.Unmarshal(reqMap["tools"], &tools); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	// Must NOT have "type" or "function" keys at top level
	if tools[0]["function"] != nil {
		t.Error("expected 'function' key to be absent in Anthropic format")
	}
	if string(tools[0]["name"]) != `"get_weather"` {
		t.Errorf("expected name='get_weather', got %s", string(tools[0]["name"]))
	}
	if tools[0]["input_schema"] == nil {
		t.Error("expected 'input_schema' key (converted from parameters)")
	}
}

func TestConvertToolsForUpstream_SameFormatNoOp(t *testing.T) {
	original := `[{"name":"x"}]`
	reqMap := map[string]json.RawMessage{
		"tools": json.RawMessage(original),
	}

	convertToolsForUpstream(reqMap, true, true)
	if string(reqMap["tools"]) != original {
		t.Error("expected no conversion when source and target are the same")
	}

	convertToolsForUpstream(reqMap, false, false)
	if string(reqMap["tools"]) != original {
		t.Error("expected no conversion when source and target are the same")
	}
}

func TestConvertToolsForUpstream_NoToolsField(t *testing.T) {
	reqMap := map[string]json.RawMessage{}
	convertToolsForUpstream(reqMap, true, false)
	// Should not panic and should not add a tools key
	if _, ok := reqMap["tools"]; ok {
		t.Error("expected no tools key to be added")
	}
}

func TestConvertToolChoiceForUpstream_AnthropicToOpenAI(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`{"type":"auto"}`, `"auto"`},
		{`{"type":"none"}`, `"none"`},
		{`{"type":"any"}`, `"required"`},
		{`{"type":"tool","name":"get_weather"}`, `{"type":"function","function":{"name":"get_weather"}}`},
	}

	for _, tt := range tests {
		reqMap := map[string]json.RawMessage{
			"tool_choice": json.RawMessage(tt.input),
		}
		convertToolChoiceForUpstream(reqMap, true, false)
		if string(reqMap["tool_choice"]) != tt.expected {
			t.Errorf("input %s: expected %s, got %s", tt.input, tt.expected, string(reqMap["tool_choice"]))
		}
	}
}

func TestConvertToolChoiceForUpstream_OpenAIToAnthropic(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"auto"`, `{"type":"auto"}`},
		{`"none"`, `{"type":"none"}`},
		{`"required"`, `{"type":"any"}`},
		{`{"type":"function","function":{"name":"get_weather"}}`, `{"type":"tool","name":"get_weather"}`},
	}

	for _, tt := range tests {
		reqMap := map[string]json.RawMessage{
			"tool_choice": json.RawMessage(tt.input),
		}
		convertToolChoiceForUpstream(reqMap, false, true)
		if string(reqMap["tool_choice"]) != tt.expected {
			t.Errorf("input %s: expected %s, got %s", tt.input, tt.expected, string(reqMap["tool_choice"]))
		}
	}
}

func TestConvertToolChoiceForUpstream_NoToolChoice(t *testing.T) {
	reqMap := map[string]json.RawMessage{}
	convertToolChoiceForUpstream(reqMap, true, false)
	if _, ok := reqMap["tool_choice"]; ok {
		t.Error("expected no tool_choice key to be added")
	}
}

// --- sanitizeForProvider tests ---

func TestSanitizeForProvider_OpenAIRemovesUnknown(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"model":       json.RawMessage(`"gpt-4"`),
		"messages":    json.RawMessage(`[]`),
		"system":      json.RawMessage(`"you are helpful"`),
		"thinking":    json.RawMessage(`{"type":"enabled"}`),
		"temperature": json.RawMessage(`0.7`),
	}
	sanitizeForProvider(reqMap, "openai")

	if _, ok := reqMap["system"]; ok {
		t.Error("expected 'system' to be removed for OpenAI provider")
	}
	if _, ok := reqMap["thinking"]; ok {
		t.Error("expected 'thinking' to be removed for OpenAI provider")
	}
	if _, ok := reqMap["model"]; !ok {
		t.Error("expected 'model' to be kept")
	}
	if _, ok := reqMap["messages"]; !ok {
		t.Error("expected 'messages' to be kept")
	}
	if _, ok := reqMap["temperature"]; !ok {
		t.Error("expected 'temperature' to be kept")
	}
}

func TestSanitizeForProvider_AnthropicRemovesUnknown(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"model":             json.RawMessage(`"claude-3"`),
		"messages":          json.RawMessage(`[]`),
		"frequency_penalty": json.RawMessage(`0.5`),
		"presence_penalty":  json.RawMessage(`0.3`),
		"logprobs":          json.RawMessage(`true`),
	}
	sanitizeForProvider(reqMap, "anthropic")

	if _, ok := reqMap["frequency_penalty"]; ok {
		t.Error("expected 'frequency_penalty' to be removed for Anthropic")
	}
	if _, ok := reqMap["presence_penalty"]; ok {
		t.Error("expected 'presence_penalty' to be removed for Anthropic")
	}
	if _, ok := reqMap["logprobs"]; ok {
		t.Error("expected 'logprobs' to be removed for Anthropic")
	}
	if _, ok := reqMap["model"]; !ok {
		t.Error("expected 'model' to be kept")
	}
}

// --- convertSystemForUpstream tests ---

func TestConvertSystem_AnthropicToOpenAI(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"system":   json.RawMessage(`"You are helpful"`),
		"messages": json.RawMessage(`[{"role":"user","content":"hi"}]`),
	}
	convertSystemForUpstream(reqMap, true, false)

	if _, ok := reqMap["system"]; ok {
		t.Error("expected 'system' to be removed when routing to OpenAI")
	}

	var msgs []map[string]string
	json.Unmarshal(reqMap["messages"], &msgs)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (system+user), got %d", len(msgs))
	}
	if msgs[0]["role"] != "system" {
		t.Errorf("expected first message role='system', got '%s'", msgs[0]["role"])
	}
	if msgs[0]["content"] != "You are helpful" {
		t.Errorf("expected system content preserved, got '%s'", msgs[0]["content"])
	}
}

func TestConvertSystem_OpenAIToAnthropic(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"messages": json.RawMessage(`[
			{"role":"system","content":"Be concise"},
			{"role":"user","content":"hi"}
		]`),
	}
	convertSystemForUpstream(reqMap, false, true)

	var sysText string
	json.Unmarshal(reqMap["system"], &sysText)
	if sysText != "Be concise" {
		t.Errorf("expected system='Be concise', got '%s'", sysText)
	}

	var msgs []map[string]string
	json.Unmarshal(reqMap["messages"], &msgs)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (user only), got %d", len(msgs))
	}
	if msgs[0]["role"] != "user" {
		t.Errorf("expected remaining message role='user', got '%s'", msgs[0]["role"])
	}
}

func TestConvertSystem_SameFormatNoOp(t *testing.T) {
	original := `"test"`
	reqMap := map[string]json.RawMessage{
		"system": json.RawMessage(original),
	}
	convertSystemForUpstream(reqMap, true, true)
	if string(reqMap["system"]) != original {
		t.Error("expected no conversion when source and target are the same")
	}
}

// --- ensureMaxTokens tests ---

func TestEnsureMaxTokens_RemovesZero(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"max_tokens": json.RawMessage(`0`),
	}
	ensureMaxTokens(reqMap, false)
	if _, ok := reqMap["max_tokens"]; ok {
		t.Error("expected max_tokens=0 to be removed")
	}
}

func TestEnsureMaxTokens_AnthropicRequired(t *testing.T) {
	reqMap := map[string]json.RawMessage{}
	ensureMaxTokens(reqMap, true)
	var mt int
	json.Unmarshal(reqMap["max_tokens"], &mt)
	if mt != 4096 {
		t.Errorf("expected max_tokens=4096 for Anthropic, got %d", mt)
	}
}

func TestEnsureMaxTokens_PreservesValid(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"max_tokens": json.RawMessage(`8192`),
	}
	ensureMaxTokens(reqMap, false)
	if string(reqMap["max_tokens"]) != `8192` {
		t.Error("expected valid max_tokens to be preserved")
	}
}

func TestStripThinkingBlocks_RemovesThinking(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"messages": json.RawMessage(`[
			{"role": "user", "content": "hello"},
			{"role": "assistant", "content": [
				{"type": "thinking", "thinking": "let me think..."},
				{"type": "text", "text": "Here is the answer"}
			]},
			{"role": "user", "content": "thanks"}
		]`),
	}

	stripThinkingBlocks(reqMap, false)

	var msgs []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	json.Unmarshal(reqMap["messages"], &msgs)

	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	// Assistant message should only have the text block
	var blocks []map[string]interface{}
	json.Unmarshal(msgs[1].Content, &blocks)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 content block after strip, got %d", len(blocks))
	}
	if blocks[0]["type"] != "text" {
		t.Errorf("expected text block, got %v", blocks[0]["type"])
	}
}

func TestStripThinkingBlocks_NoOpForAnthropic(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"messages": json.RawMessage(`[
			{"role": "assistant", "content": [
				{"type": "thinking", "thinking": "hmm"},
				{"type": "text", "text": "answer"}
			]}
		]`),
	}

	original := string(reqMap["messages"])
	stripThinkingBlocks(reqMap, true)

	// Must be completely unchanged for Anthropic target
	if string(reqMap["messages"]) != original {
		t.Error("expected messages to be unchanged for Anthropic target")
	}
}

func TestStripThinkingBlocks_StringContentNoOp(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"messages": json.RawMessage(`[
			{"role": "user", "content": "just a string"}
		]`),
	}

	stripThinkingBlocks(reqMap, false)

	var msgs []struct {
		Content string `json:"content"`
	}
	json.Unmarshal(reqMap["messages"], &msgs)
	if msgs[0].Content != "just a string" {
		t.Error("expected string content to be unchanged")
	}
}

func TestEnsureMaxTokens_CompletionTokensFallback(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"max_completion_tokens": json.RawMessage(`2048`),
	}
	ensureMaxTokens(reqMap, false)
	if string(reqMap["max_tokens"]) != `2048` {
		t.Errorf("expected max_tokens from max_completion_tokens, got %s", string(reqMap["max_tokens"]))
	}
	if _, ok := reqMap["max_completion_tokens"]; ok {
		t.Error("expected max_completion_tokens to be removed")
	}
}

type passthroughTestRouter struct {
	info  router.ProviderInfo
	found bool
}

func (r passthroughTestRouter) Route(ctx context.Context, req *router.RouteRequest) *router.RouteResult {
	return nil
}

func (r passthroughTestRouter) ResolveProvider(modelRef string) (router.ProviderInfo, bool) {
	return r.info, r.found
}

type selectionExplainerRouter struct {
	model       *router.ModelConfig
	complexity  int64
	diagnostics *router.RouteDiagnostics
}

func (r selectionExplainerRouter) Route(ctx context.Context, req *router.RouteRequest) *router.RouteResult {
	return nil
}

func (r selectionExplainerRouter) SelectModel(ctx context.Context, req *router.RouteRequest) (*router.ModelConfig, int64, string) {
	if r.model == nil {
		return nil, 0, "no model"
	}
	return r.model, r.complexity, ""
}

func (r selectionExplainerRouter) ExplainSelection(ctx context.Context, req *router.RouteRequest) *router.SelectionResult {
	if r.model == nil {
		return &router.SelectionResult{ErrorMsg: "no model", Diagnostics: r.diagnostics}
	}
	return &router.SelectionResult{
		Model:       r.model,
		Complexity:  r.complexity,
		Diagnostics: r.diagnostics,
	}
}

func TestBuildUpstreamURL_StripsProxyV1PrefixAndPreservesQuery(t *testing.T) {
	got := buildUpstreamURL("https://api.openai.com/v1", "/v1/responses", "include=reasoning.encrypted_content")
	want := "https://api.openai.com/v1/responses?include=reasoning.encrypted_content"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestRewritePassthroughModel_ReplacesProxyModelReference(t *testing.T) {
	body := []byte(`{"model":"auto","input":"hello"}`)
	rewritten := rewritePassthroughModel(body, "gpt-4.1-mini")

	var payload map[string]string
	if err := json.Unmarshal(rewritten, &payload); err != nil {
		t.Fatalf("expected valid JSON, got %v", err)
	}
	if payload["model"] != "gpt-4.1-mini" {
		t.Fatalf("expected rewritten model, got %q", payload["model"])
	}
}

func TestHandlePassthrough_OpenAIResponsesUsesUpstreamAPIRoot(t *testing.T) {
	var gotPath string
	var gotQuery string
	var gotAuth string
	var gotModel string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if err := json.Unmarshal(payload["model"], &gotModel); err != nil {
			t.Fatalf("decode model: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resp_123","object":"response"}`))
	}))
	defer upstream.Close()

	s := &Server{
		router: passthroughTestRouter{
			info: router.ProviderInfo{
				BaseURL:  upstream.URL + "/v1",
				APIKey:   "upstream-secret",
				ModelID:  "gpt-4.1-mini",
				Provider: "openai",
			},
			found: true,
		},
		httpClient: upstream.Client(),
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/responses?include=reasoning.encrypted_content", bytes.NewBufferString(`{"model":"auto","input":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer local-proxy-key")

	rr := httptest.NewRecorder()
	s.handlePassthrough(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if gotPath != "/v1/responses" {
		t.Fatalf("expected upstream path /v1/responses, got %q", gotPath)
	}
	if gotQuery != "include=reasoning.encrypted_content" {
		t.Fatalf("expected query to be preserved, got %q", gotQuery)
	}
	if gotAuth != "Bearer upstream-secret" {
		t.Fatalf("expected upstream auth header to be rewritten, got %q", gotAuth)
	}
	if gotModel != "gpt-4.1-mini" {
		t.Fatalf("expected upstream model to be rewritten, got %q", gotModel)
	}
	if rr.Body.String() != `{"id":"resp_123","object":"response"}` {
		t.Fatalf("unexpected passthrough body: %s", rr.Body.String())
	}
}

func TestHandleChatCompletion_SetsDecisionHeaderFromDiagnostics(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_123","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer upstream.Close()

	model := &router.ModelConfig{
		ID:       "m1",
		Name:     "Fast",
		Provider: "openai",
		BaseURL:  upstream.URL,
		APIKey:   "secret",
		ModelID:  "gpt-4.1-mini",
	}
	diagnostics := &router.RouteDiagnostics{
		Mode:          "auto",
		SelectedModel: "Fast",
		Summary:       "mode=auto; selected=Fast; eligible=2; skipped=1",
	}

	s := &Server{
		router: selectionExplainerRouter{
			model:       model,
			complexity:  1,
			diagnostics: diagnostics,
		},
		httpClient: upstream.Client(),
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"auto","messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleChatCompletion(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("X-Router-Decision"); got != diagnostics.Summary {
		t.Fatalf("expected decision header %q, got %q", diagnostics.Summary, got)
	}
}

func TestHandleChatCompletion_ConvertsOpenAINonStreamResponseForAnthropicClient(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("expected OpenAI upstream path, got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_123","object":"chat.completion","model":"gpt-4.1-mini","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`))
	}))
	defer upstream.Close()

	model := &router.ModelConfig{
		ID:       "m1",
		Name:     "Fast",
		Provider: "openai",
		BaseURL:  upstream.URL,
		APIKey:   "secret",
		ModelID:  "gpt-4.1-mini",
	}
	s := &Server{router: selectionExplainerRouter{model: model}, httpClient: upstream.Client()}

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(`{"model":"auto","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleChatCompletion(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("expected valid Anthropic JSON, got %v body=%s", err, rr.Body.String())
	}
	if got.Type != "message" || got.Role != "assistant" {
		t.Fatalf("expected Anthropic message response, got type=%q role=%q", got.Type, got.Role)
	}
	if len(got.Content) != 1 || got.Content[0].Type != "text" || got.Content[0].Text != "ok" {
		t.Fatalf("unexpected content: %+v", got.Content)
	}
	if got.Usage.InputTokens != 7 || got.Usage.OutputTokens != 2 {
		t.Fatalf("unexpected usage: %+v", got.Usage)
	}
}

func TestHandleChatCompletion_ConvertsOpenAIStreamResponseForAnthropicClient(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}],\"model\":\"gpt-4.1-mini\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"model\":\"gpt-4.1-mini\",\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":2}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	model := &router.ModelConfig{
		ID:       "m1",
		Name:     "Fast",
		Provider: "openai",
		BaseURL:  upstream.URL,
		APIKey:   "secret",
		ModelID:  "gpt-4.1-mini",
	}
	s := &Server{router: selectionExplainerRouter{model: model}, httpClient: upstream.Client()}

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(`{"model":"auto","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleChatCompletion(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "event: message_start") || !strings.Contains(body, "event: content_block_delta") || !strings.Contains(body, "event: message_stop") {
		t.Fatalf("expected Anthropic SSE events, got %s", body)
	}
	if !strings.Contains(body, `"text":"ok"`) {
		t.Fatalf("expected converted text delta, got %s", body)
	}
}

func TestSanitizeNullContent_ReplacesNull(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"messages": json.RawMessage(`[
			{"role": "user", "content": "hello"},
			{"role": "assistant", "content": null, "tool_calls": [{"id": "c1", "type": "function", "function": {"name": "get_weather"}}]},
			{"role": "tool", "content": "sunny"}
		]`),
	}

	sanitizeNullContent(reqMap)

	var msgs []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	json.Unmarshal(reqMap["messages"], &msgs)

	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if string(msgs[1].Content) != `""` {
		t.Errorf("expected null content replaced with empty string, got %s", string(msgs[1].Content))
	}
	// Non-null content should be unchanged
	if string(msgs[0].Content) != `"hello"` {
		t.Errorf("expected user content unchanged, got %s", string(msgs[0].Content))
	}
}

func TestSanitizeNullContent_NoNulls(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"messages": json.RawMessage(`[
			{"role": "user", "content": "hello"},
			{"role": "assistant", "content": "hi there"}
		]`),
	}

	original := string(reqMap["messages"])
	sanitizeNullContent(reqMap)

	if string(reqMap["messages"]) != original {
		t.Error("expected messages unchanged when no null content")
	}
}

func TestSanitizeNullContent_NoMessages(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"model": json.RawMessage(`"gpt-4"`),
	}

	sanitizeNullContent(reqMap)

	if _, ok := reqMap["messages"]; ok {
		t.Error("expected no messages key to remain absent")
	}
}
