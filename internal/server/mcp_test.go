package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The MCP protocol itself is tested in llama-swap. These check that the
// site mounts docbot with the source archive.

type toolResult struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// mcpCall posts a JSON-RPC request with the MCP headers and decodes the result.
func mcpCall(t *testing.T, s *Server, method string, params map[string]any, out any) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	req := httptest.NewRequest(http.MethodPost, "/api/mcp", strings.NewReader(string(raw)))
	req.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	req.Header.Set("Mcp-Method", method)
	if name, ok := params["name"].(string); ok {
		req.Header.Set("Mcp-Name", name)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	var reply struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
		t.Fatalf("%s: HTTP %d: %s", method, w.Code, w.Body.String())
	}
	if reply.Error != nil {
		t.Fatalf("%s: %s", method, reply.Error.Message)
	}
	if err := json.Unmarshal(reply.Result, out); err != nil {
		t.Fatal(err)
	}
}

func TestMCP_ToolsList(t *testing.T) {
	s := newTestServer(t)
	var result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	mcpCall(t, s, "tools/list", nil, &result)

	var names []string
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	want := "code__find code__grep code__ls code__read docs__get_config_schema docs__get_doc docs__list_docs docs__search_docs"
	if got := strings.Join(names, " "); got != want {
		t.Errorf("tools = %s\nwant    %s", got, want)
	}
}

func TestMCP_SearchDocs(t *testing.T) {
	s := newTestServer(t)
	var result toolResult
	mcpCall(t, s, "tools/call", map[string]any{"name": "docs__search_docs", "arguments": map[string]any{"query": "ttl unload"}}, &result)
	if result.IsError || !strings.Contains(result.Content[0].Text, "ttl") {
		t.Errorf("search result: %+v", result)
	}
}

// code__read must find the embedded archive and link to the llama-swap
// commit in go.mod.
func TestMCP_CodeReadUsesEmbeddedSource(t *testing.T) {
	s := newTestServer(t)
	var result toolResult
	mcpCall(t, s, "tools/call", map[string]any{"name": "code__read", "arguments": map[string]any{"path": "docbot/docbot.go"}}, &result)
	text := result.Content[0].Text
	if result.IsError || !strings.Contains(text, "package docbot") {
		t.Fatalf("code__read: %.300s", text)
	}
	if !strings.Contains(text, "github.com/mostlygeek/llama-swap/blob/") || strings.Contains(text, "/blob/main/") {
		t.Errorf("code__read should link to a pinned commit: %.400s", text)
	}
}

func TestMCP_RejectsGET(t *testing.T) {
	s := newTestServer(t)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/mcp", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/mcp = %d", w.Code)
	}
}
