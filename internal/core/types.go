package core

// ModelJSON is the frontend-safe representation of a model (no API key).
type ModelJSON struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Provider       string `json:"provider"`
	BaseURL        string `json:"baseUrl"`
	APIKey         string `json:"apiKey"`
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

// ChatMessage represents a single message in a chat request.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is a chat request from the frontend.
type ChatRequest struct {
	Messages []ChatMessage `json:"messages"`
	Mode     string        `json:"mode"`
	ModelID  string        `json:"modelId"`
}

// ChatResponse is sent back after routing a chat.
type ChatResponse struct {
	ModelID         string `json:"modelId"`
	ModelName       string `json:"modelName"`
	Provider        string `json:"provider"`
	Complexity      string `json:"complexity"`
	RouteMode       string `json:"routeMode"`
	Status          string `json:"status"`
	Error           string `json:"error"`
	Diagnostics     string `json:"diagnostics"`
	DiagnosticsJSON string `json:"diagnosticsJson"`
}

// ProxyStatus represents the current state of the proxy server.
type ProxyStatus struct {
	Running bool
	Port    int
	Mode    string
}
