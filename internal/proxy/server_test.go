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
	"time"

	"multi_model_router/internal/router"
)

func TestServerStart_AcceptsOpenAIPathWithoutV1(t *testing.T) {
	selected := &router.ModelConfig{ID: "selected", Name: "Selected", Provider: "openai", BaseURL: "http://127.0.0.1", APIKey: "secret", ModelID: "gpt-selected"}
	s := NewWithManualModel(0, selectionExplainerRouter{model: selected}, router.RouteManual, "selected", "")
	if s.server != nil {
		t.Fatal("expected server not started")
	}

	req := httptest.NewRequest(http.MethodPost, "/chat/completions", bytes.NewBufferString(`{"model":"auto","messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletion)
	mux.HandleFunc("/chat/completions", s.handleChatCompletion)
	mux.ServeHTTP(rr, req)

	if rr.Code == http.StatusOK && strings.Contains(rr.Body.String(), `"service":"multi-model-router"`) {
		t.Fatalf("expected /chat/completions not to route to health handler, got %s", rr.Body.String())
	}
}

func TestConvertMessagesForUpstream_OpenAIToolMessageToAnthropicUserToolResult(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"messages": json.RawMessage(`[
			{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},
			{"role":"tool","tool_call_id":"call_1","content":"sunny"}
		]`),
	}

	convertMessagesForUpstream(reqMap, false, true)

	var msgs []struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCallID string          `json:"tool_call_id"`
	}
	if err := json.Unmarshal(reqMap["messages"], &msgs); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[1].Role != "user" {
		t.Fatalf("expected tool message role converted to user, got %q", msgs[1].Role)
	}
	if msgs[1].ToolCallID != "" {
		t.Fatalf("expected tool_call_id removed from Anthropic message, got %q", msgs[1].ToolCallID)
	}
	var content []struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal(msgs[1].Content, &content); err != nil {
		t.Fatalf("decode tool_result content: %v", err)
	}
	if len(content) != 1 || content[0].Type != "tool_result" || content[0].ToolUseID != "call_1" || content[0].Content != "sunny" {
		t.Fatalf("unexpected tool_result content: %+v", content)
	}
}

func TestConvertMessagesForUpstream_OpenAIToolCallsToAnthropicToolUse(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"messages": json.RawMessage(`[
			{"role":"assistant","content":"checking","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"city\":\"Paris\"}"}}]}
		]`),
	}

	convertMessagesForUpstream(reqMap, false, true)

	var msgs []struct {
		Role      string          `json:"role"`
		Content   json.RawMessage `json:"content"`
		ToolCalls json.RawMessage `json:"tool_calls"`
	}
	if err := json.Unmarshal(reqMap["messages"], &msgs); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Role != "assistant" {
		t.Fatalf("unexpected messages: %+v", msgs)
	}
	if len(msgs[0].ToolCalls) != 0 {
		t.Fatalf("expected tool_calls removed, got %s", string(msgs[0].ToolCalls))
	}
	var blocks []struct {
		Type  string         `json:"type"`
		Text  string         `json:"text"`
		ID    string         `json:"id"`
		Name  string         `json:"name"`
		Input map[string]any `json:"input"`
	}
	if err := json.Unmarshal(msgs[0].Content, &blocks); err != nil {
		t.Fatalf("decode content blocks: %v", err)
	}
	if len(blocks) != 2 || blocks[0].Type != "text" || blocks[0].Text != "checking" {
		t.Fatalf("expected leading text block, got %+v", blocks)
	}
	if blocks[1].Type != "tool_use" || blocks[1].ID != "call_1" || blocks[1].Name != "lookup" || blocks[1].Input["city"] != "Paris" {
		t.Fatalf("unexpected tool_use block: %+v", blocks[1])
	}
}

func TestConvertMessagesForUpstream_OpenAIImageToAnthropicImage(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"messages": json.RawMessage(`[
			{"role":"user","content":[
				{"type":"text","text":"describe"},
				{"type":"image_url","image_url":{"url":"data:image/png;base64,abc123"}}
			]}
		]`),
	}

	convertMessagesForUpstream(reqMap, false, true)

	var msgs []struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(reqMap["messages"], &msgs); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	var blocks []struct {
		Type   string `json:"type"`
		Source struct {
			Type      string `json:"type"`
			MediaType string `json:"media_type"`
			Data      string `json:"data"`
		} `json:"source"`
	}
	if err := json.Unmarshal(msgs[0].Content, &blocks); err != nil {
		t.Fatalf("decode content blocks: %v", err)
	}
	if len(blocks) != 2 || blocks[1].Type != "image" || blocks[1].Source.Type != "base64" || blocks[1].Source.MediaType != "image/png" || blocks[1].Source.Data != "abc123" {
		t.Fatalf("unexpected Anthropic image block: %+v", blocks)
	}
}

func TestConvertMessagesForUpstream_AnthropicToolUseAndResultToOpenAI(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"messages": json.RawMessage(`[
			{"role":"assistant","content":[
				{"type":"text","text":"checking"},
				{"type":"tool_use","id":"call_1","name":"lookup","input":{"city":"Paris"}}
			]},
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"call_1","content":"sunny"}
			]}
		]`),
	}

	convertMessagesForUpstream(reqMap, true, false)

	var msgs []struct {
		Role       string          `json:"role"`
		Content    string          `json:"content"`
		ToolCalls  json.RawMessage `json:"tool_calls"`
		ToolCallID string          `json:"tool_call_id"`
	}
	if err := json.Unmarshal(reqMap["messages"], &msgs); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 OpenAI messages, got %+v", msgs)
	}
	if msgs[0].Role != "assistant" || msgs[0].Content != "checking" || len(msgs[0].ToolCalls) == 0 {
		t.Fatalf("unexpected assistant conversion: %+v", msgs[0])
	}
	if msgs[1].Role != "tool" || msgs[1].ToolCallID != "call_1" || msgs[1].Content != "sunny" {
		t.Fatalf("unexpected tool_result conversion: %+v", msgs[1])
	}
}

func TestPreserveCompletionTokenLimit_OpenAIToAnthropic(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"max_completion_tokens": json.RawMessage(`1234`),
	}
	preserveCompletionTokenLimit(reqMap, true)
	if string(reqMap["max_tokens"]) != `1234` {
		t.Fatalf("expected max_tokens copied from max_completion_tokens, got %s", string(reqMap["max_tokens"]))
	}
}

func TestConvertRequestParameters_OpenAIToAnthropic(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"stop": json.RawMessage(`["END"]`),
		"user": json.RawMessage(`"user_123"`),
	}

	convertRequestParametersForUpstream(reqMap, false, true)

	if string(reqMap["stop_sequences"]) != `["END"]` {
		t.Fatalf("expected stop mapped to stop_sequences, got %s", string(reqMap["stop_sequences"]))
	}
	if _, ok := reqMap["stop"]; ok {
		t.Fatal("expected OpenAI stop field removed for Anthropic target")
	}
	var metadata map[string]string
	if err := json.Unmarshal(reqMap["metadata"], &metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if metadata["user_id"] != "user_123" {
		t.Fatalf("expected user mapped to metadata.user_id, got %+v", metadata)
	}
}

func TestConvertRequestParameters_AnthropicToOpenAI(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"stop_sequences": json.RawMessage(`["END"]`),
		"metadata":       json.RawMessage(`{"user_id":"user_123"}`),
	}

	convertRequestParametersForUpstream(reqMap, true, false)

	if string(reqMap["stop"]) != `["END"]` {
		t.Fatalf("expected stop_sequences mapped to stop, got %s", string(reqMap["stop"]))
	}
	if _, ok := reqMap["stop_sequences"]; ok {
		t.Fatal("expected Anthropic stop_sequences field removed for OpenAI target")
	}
	if string(reqMap["user"]) != `"user_123"` {
		t.Fatalf("expected metadata.user_id mapped to user, got %s", string(reqMap["user"]))
	}
}

func TestHandleChatCompletion_ManualModeUsesConfiguredModel(t *testing.T) {
	var upstreamModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		upstreamModel = payload.Model
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_123","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer upstream.Close()

	selected := &router.ModelConfig{ID: "selected", Name: "Selected", Provider: "openai", BaseURL: upstream.URL, APIKey: "secret", ModelID: "gpt-selected"}
	s := &Server{router: selectionExplainerRouter{model: selected}, defaultMode: router.RouteManual, manualModelID: "selected", httpClient: upstream.Client()}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"auto","messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleChatCompletion(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if upstreamModel != "gpt-selected" {
		t.Fatalf("expected configured manual model sent upstream, got %q", upstreamModel)
	}
}

func TestNewWithManualModel_UsesLongProxyTimeout(t *testing.T) {
	s := NewWithManualModel(9680, nil, router.RouteAuto, "", "")
	if s.httpClient.Timeout != 30*time.Minute {
		t.Fatalf("expected 30 minute timeout, got %s", s.httpClient.Timeout)
	}
}
func TestInjectStreamOptions_OnlyForStreaming(t *testing.T) {
	nonStreaming := []byte(`{"model":"gpt","stream":false}`)
	if got := injectStreamOptions(nonStreaming); string(got) != string(nonStreaming) {
		t.Fatalf("expected non-streaming request unchanged, got %s", string(got))
	}

	streaming := injectStreamOptions([]byte(`{"model":"gpt","stream":true}`))
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(streaming, &payload); err != nil {
		t.Fatalf("decode streaming payload: %v", err)
	}
	if _, ok := payload["stream_options"]; !ok {
		t.Fatalf("expected stream_options injected, got %s", string(streaming))
	}
}

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

func TestSanitizeThinkingForProtocol_OpenAIClientToAnthropicPreservesThinking(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"thinking": json.RawMessage(`{"type":"enabled","budget_tokens":1024}`),
		"messages": json.RawMessage(`[
			{"role": "assistant", "content": [
				{"type": "thinking", "thinking": "hidden chain", "signature": "sig"},
				{"type": "text", "text": "visible answer"}
			]},
			{"role": "user", "content": "continue"}
		]`),
	}
	originalMessages := string(reqMap["messages"])

	sanitizeThinkingForProtocol(reqMap, false, true)

	if _, ok := reqMap["thinking"]; !ok {
		t.Fatal("expected top-level thinking to be preserved for OpenAI-compatible clients")
	}
	if string(reqMap["messages"]) != originalMessages {
		t.Fatalf("expected thinking content blocks to be preserved, got %s", string(reqMap["messages"]))
	}
}

func TestSanitizeThinkingForProtocol_AnthropicClientPreservesThinking(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"thinking": json.RawMessage(`{"type":"enabled","budget_tokens":1024}`),
		"messages": json.RawMessage(`[
			{"role": "assistant", "content": [
				{"type": "thinking", "thinking": "hidden chain", "signature": "sig"},
				{"type": "text", "text": "visible answer"}
			]}
		]`),
	}
	originalMessages := string(reqMap["messages"])

	sanitizeThinkingForProtocol(reqMap, true, true)

	if _, ok := reqMap["thinking"]; !ok {
		t.Fatal("expected top-level thinking to be preserved for Anthropic clients")
	}
	if string(reqMap["messages"]) != originalMessages {
		t.Fatalf("expected Anthropic message content to be unchanged, got %s", string(reqMap["messages"]))
	}
}

func TestSanitizeThinkingForProtocol_AnthropicClientPreservesIncompleteThinkingHistory(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"thinking": json.RawMessage(`{"type":"enabled","budget_tokens":1024}`),
		"messages": json.RawMessage(`[
			{"role": "assistant", "content": [{"type": "text", "text": "visible answer"}]},
			{"role": "user", "content": "continue"}
		]`),
	}

	sanitizeThinkingForProtocol(reqMap, true, true)

	if _, ok := reqMap["thinking"]; !ok {
		t.Fatal("expected thinking to be preserved even when existing history is incomplete")
	}
}

func TestSanitizeThinkingForProtocol_AnthropicClientPreservesRedactedThinking(t *testing.T) {
	reqMap := map[string]json.RawMessage{
		"thinking": json.RawMessage(`{"type":"enabled","budget_tokens":1024}`),
		"messages": json.RawMessage(`[
			{"role": "assistant", "content": [
				{"type": "redacted_thinking", "data": "encrypted"},
				{"type": "text", "text": "visible answer"}
			]}
		]`),
	}
	originalMessages := string(reqMap["messages"])

	sanitizeThinkingForProtocol(reqMap, true, true)

	if _, ok := reqMap["thinking"]; !ok {
		t.Fatal("expected top-level thinking to be preserved with redacted thinking history")
	}
	if string(reqMap["messages"]) != originalMessages {
		t.Fatalf("expected Anthropic message content to be unchanged, got %s", string(reqMap["messages"]))
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

func TestHandleChatCompletion_OpenAIClientToAnthropicReturnsStringContent(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Fatalf("expected Anthropic upstream path, got %q", r.URL.Path)
		}
		var payload map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if _, ok := payload["thinking"]; !ok {
			t.Fatal("expected top-level thinking to be preserved for OpenAI-compatible client")
		}
		var msgs []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(payload["messages"], &msgs); err != nil {
			t.Fatalf("decode upstream messages: %v", err)
		}
		var blocks []struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(msgs[0].Content, &blocks); err != nil {
			t.Fatalf("decode assistant content blocks: %v", err)
		}
		hasThinking := false
		for _, block := range blocks {
			if block.Type == "thinking" {
				hasThinking = true
			}
		}
		if !hasThinking {
			t.Fatalf("expected thinking content blocks to be preserved, got %+v", blocks)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"msg_123","type":"message","role":"assistant","model":"claude-test","content":[{"type":"thinking","thinking":"new hidden chain","signature":"new_sig"},{"type":"text","text":"ok"}],"usage":{"input_tokens":7,"output_tokens":2}}`))
	}))
	defer upstream.Close()

	model := &router.ModelConfig{
		ID:       "m1",
		Name:     "Claude",
		Provider: "anthropic",
		BaseURL:  upstream.URL,
		APIKey:   "secret",
		ModelID:  "claude-test",
	}
	s := &Server{router: selectionExplainerRouter{model: model}, httpClient: upstream.Client()}

	body := `{
		"model":"auto",
		"max_tokens":64,
		"thinking":{"type":"enabled","budget_tokens":1024},
		"messages":[
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"hidden chain","signature":"sig"},
				{"type":"text","text":"visible answer"}
			]},
			{"role":"user","content":"continue"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleChatCompletion(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("expected OpenAI-compatible JSON with string content, got %v body=%s", err, rr.Body.String())
	}
	if len(got.Choices) != 1 {
		t.Fatalf("unexpected converted response: %+v", got)
	}
	if got.Choices[0].Message.Content != "ok" {
		t.Fatalf("expected OpenAI response content to contain text only, got %q", got.Choices[0].Message.Content)
	}
	if strings.Contains(got.Choices[0].Message.Content, "thinking") || strings.Contains(got.Choices[0].Message.Content, "new hidden chain") {
		t.Fatalf("expected thinking omitted from OpenAI response content, got %q", got.Choices[0].Message.Content)
	}
}

func TestConvertNonStreamingResponse_AnthropicToolUseToOpenAIToolCalls(t *testing.T) {
	body := []byte(`{"id":"msg_123","type":"message","role":"assistant","model":"claude-test","stop_reason":"tool_use","content":[{"type":"text","text":"checking"},{"type":"tool_use","id":"call_1","name":"lookup","input":{"city":"Paris"}}],"usage":{"input_tokens":7,"output_tokens":2}}`)

	converted, err := convertNonStreamingResponse(body, true, "Claude")
	if err != nil {
		t.Fatalf("convert response: %v", err)
	}
	var got struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(converted, &got); err != nil {
		t.Fatalf("decode converted response: %v body=%s", err, string(converted))
	}
	if got.Choices[0].FinishReason != "tool_calls" || got.Choices[0].Message.Content != "checking" {
		t.Fatalf("unexpected converted choice: %+v", got.Choices[0])
	}
	if len(got.Choices[0].Message.ToolCalls) != 1 || got.Choices[0].Message.ToolCalls[0].ID != "call_1" || got.Choices[0].Message.ToolCalls[0].Function.Name != "lookup" || !strings.Contains(got.Choices[0].Message.ToolCalls[0].Function.Arguments, "Paris") {
		t.Fatalf("unexpected tool calls: %+v", got.Choices[0].Message.ToolCalls)
	}
}

func TestConvertNonStreamingResponse_OpenAIToolCallsToAnthropicToolUse(t *testing.T) {
	body := []byte(`{"id":"chatcmpl_123","object":"chat.completion","model":"gpt","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":"checking","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"city\":\"Paris\"}"}}]}}],"usage":{"prompt_tokens":7,"completion_tokens":2}}`)

	converted, err := convertNonStreamingResponse(body, false, "GPT")
	if err != nil {
		t.Fatalf("convert response: %v", err)
	}
	var got struct {
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type  string         `json:"type"`
			Text  string         `json:"text"`
			ID    string         `json:"id"`
			Name  string         `json:"name"`
			Input map[string]any `json:"input"`
		} `json:"content"`
	}
	if err := json.Unmarshal(converted, &got); err != nil {
		t.Fatalf("decode converted response: %v body=%s", err, string(converted))
	}
	if got.StopReason != "tool_use" || len(got.Content) != 2 || got.Content[0].Text != "checking" {
		t.Fatalf("unexpected Anthropic response: %+v", got)
	}
	if got.Content[1].Type != "tool_use" || got.Content[1].ID != "call_1" || got.Content[1].Name != "lookup" || got.Content[1].Input["city"] != "Paris" {
		t.Fatalf("unexpected tool_use block: %+v", got.Content[1])
	}
}

func TestHandleChatCompletion_OpenAIClientToAnthropicStreamReturnsStringDeltas(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Fatalf("expected Anthropic upstream path, got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-test\",\"usage\":{\"input_tokens\":7}}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"hidden\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"signature_delta\",\"signature\":\"sig\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":2}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer upstream.Close()

	model := &router.ModelConfig{
		ID:       "m1",
		Name:     "Claude",
		Provider: "anthropic",
		BaseURL:  upstream.URL,
		APIKey:   "secret",
		ModelID:  "claude-test",
	}
	s := &Server{router: selectionExplainerRouter{model: model}, httpClient: upstream.Client()}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"auto","stream":true,"max_tokens":64,"thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleChatCompletion(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, `"type":"thinking"`) || strings.Contains(body, `"thinking":"hidden"`) || strings.Contains(body, `"signature":"sig"`) {
		t.Fatalf("expected OpenAI stream to omit thinking content blocks, got %s", body)
	}
	if !strings.Contains(body, `"delta":{"content":"ok"}`) {
		t.Fatalf("expected OpenAI stream to include string text delta, got %s", body)
	}
}

func TestHandleChatCompletion_AnthropicClientToAnthropicPreservesThinking(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Fatalf("expected Anthropic upstream path, got %q", r.URL.Path)
		}
		var payload map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if _, ok := payload["thinking"]; !ok {
			t.Fatal("expected top-level thinking to be preserved for Anthropic clients")
		}
		var msgs []struct {
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(payload["messages"], &msgs); err != nil {
			t.Fatalf("decode upstream messages: %v", err)
		}
		var blocks []struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(msgs[0].Content, &blocks); err != nil {
			t.Fatalf("decode assistant content blocks: %v", err)
		}
		hasThinking := false
		for _, block := range blocks {
			if block.Type == "thinking" {
				hasThinking = true
			}
		}
		if !hasThinking {
			t.Fatalf("expected thinking content block to be preserved, got %+v", blocks)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"msg_123","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":7,"output_tokens":2}}`))
	}))
	defer upstream.Close()

	model := &router.ModelConfig{
		ID:       "m1",
		Name:     "Claude",
		Provider: "anthropic",
		BaseURL:  upstream.URL,
		APIKey:   "secret",
		ModelID:  "claude-test",
	}
	s := &Server{router: selectionExplainerRouter{model: model}, httpClient: upstream.Client()}

	body := `{
		"model":"auto",
		"max_tokens":64,
		"thinking":{"type":"enabled","budget_tokens":1024},
		"messages":[
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"hidden chain","signature":"sig"},
				{"type":"text","text":"visible answer"}
			]},
			{"role":"user","content":"continue"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleChatCompletion(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"type":"message"`) {
		t.Fatalf("expected native Anthropic response passthrough, got %s", rr.Body.String())
	}
}

func TestHandleChatCompletion_AnthropicClientToAnthropicPreservesIncompleteThinkingHistory(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Fatalf("expected Anthropic upstream path, got %q", r.URL.Path)
		}
		var payload map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if _, ok := payload["thinking"]; !ok {
			t.Fatal("expected top-level thinking to be preserved")
		}
		var msgs []struct {
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(payload["messages"], &msgs); err != nil {
			t.Fatalf("decode upstream messages: %v", err)
		}
		var blocks []struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(msgs[0].Content, &blocks); err != nil {
			t.Fatalf("decode assistant content blocks: %v", err)
		}
		if len(blocks) != 1 || blocks[0].Type != "text" {
			t.Fatalf("expected existing text-only content to be preserved, got %+v", blocks)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"msg_123","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":7,"output_tokens":2}}`))
	}))
	defer upstream.Close()

	model := &router.ModelConfig{
		ID:       "m1",
		Name:     "Claude",
		Provider: "anthropic",
		BaseURL:  upstream.URL,
		APIKey:   "secret",
		ModelID:  "claude-test",
	}
	s := &Server{router: selectionExplainerRouter{model: model}, httpClient: upstream.Client()}

	body := `{
		"model":"auto",
		"max_tokens":64,
		"thinking":{"type":"enabled","budget_tokens":1024},
		"messages":[
			{"role":"assistant","content":[{"type":"text","text":"visible answer"}]},
			{"role":"user","content":"continue"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleChatCompletion(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr.Code, rr.Body.String())
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

func TestHandleChatCompletion_ConvertsOpenAIStreamToolCallsForAnthropicClient(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("expected OpenAI upstream path, got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"lookup\",\"arguments\":\"{\\\"city\\\"\"}}]},\"finish_reason\":null}],\"model\":\"gpt-4.1-mini\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\":\\\"Paris\\\"}\"}}]},\"finish_reason\":null}],\"model\":\"gpt-4.1-mini\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}],\"model\":\"gpt-4.1-mini\",\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":2}}\n\n"))
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

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(`{"model":"auto","stream":true,"max_tokens":64,"tools":[{"name":"lookup","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleChatCompletion(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"event: content_block_start",
		`"type":"tool_use"`,
		`"id":"call_1"`,
		`"name":"lookup"`,
		`"type":"input_json_delta"`,
		`"stop_reason":"tool_use"`,
		"event: message_stop",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected converted Anthropic tool stream to contain %s, got %s", want, body)
		}
	}
}

func TestHandleChatCompletion_ConvertsAnthropicStreamToolUseForOpenAIClient(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Fatalf("expected Anthropic upstream path, got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-test\",\"usage\":{\"input_tokens\":7}}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"call_1\",\"name\":\"lookup\",\"input\":{}}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"city\\\"\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\":\\\"Paris\\\"}\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":2}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer upstream.Close()

	model := &router.ModelConfig{
		ID:       "m1",
		Name:     "Claude",
		Provider: "anthropic",
		BaseURL:  upstream.URL,
		APIKey:   "secret",
		ModelID:  "claude-test",
	}
	s := &Server{router: selectionExplainerRouter{model: model}, httpClient: upstream.Client()}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"auto","stream":true,"max_tokens":64,"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}],"messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	s.handleChatCompletion(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		`"tool_calls"`,
		`"id":"call_1"`,
		`"name":"lookup"`,
		`"arguments":"{\"city\""`,
		`"arguments":":\"Paris\"}"`,
		`"finish_reason":"tool_calls"`,
		"data: [DONE]",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected converted OpenAI tool stream to contain %s, got %s", want, body)
		}
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

func TestParseAnthropicNonStreamingForOpenAI_ContentIsStringWithoutThinkingBlocks(t *testing.T) {
	body := []byte(`{
		"content": [
			{"type":"thinking","thinking":"internal reasoning","signature":"sig"},
			{"type":"text","text":"HEARTBEAT_OK"}
		],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 2, "output_tokens": 3}
	}`)

	message, usage, finishReason, err := parseAnthropicNonStreamingForOpenAI(body)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	content, ok := message["content"].(string)
	if !ok {
		t.Fatalf("expected OpenAI message content to be string, got %T: %#v", message["content"], message["content"])
	}
	if content != "HEARTBEAT_OK" {
		t.Fatalf("expected text content only, got %q", content)
	}
	if strings.Contains(content, "thinking") || strings.Contains(content, "internal reasoning") {
		t.Fatalf("expected thinking block omitted from OpenAI content, got %q", content)
	}
	if finishReason != "stop" {
		t.Fatalf("expected finish reason stop, got %q", finishReason)
	}
	if usage == nil || usage.InputTokens != 2 || usage.OutputTokens != 3 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

func TestTextFromContentBlock_OnlyExtractsTextBlocks(t *testing.T) {
	if got := textFromContentBlock(map[string]interface{}{"type": "text", "text": "hello"}); got != "hello" {
		t.Fatalf("expected text block extracted, got %q", got)
	}
	if got := textFromContentBlock(map[string]interface{}{"type": "thinking", "thinking": "secret"}); got != "" {
		t.Fatalf("expected thinking block omitted, got %q", got)
	}
}
