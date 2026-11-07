package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type rpcReply struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

// mcpCall posts a JSON-RPC request with the standard MCP headers.
func mcpCall(t *testing.T, s *Server, method string, params any, headers map[string]string) rpcReply {
	t.Helper()
	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		body["params"] = params
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/mcp", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerMCPProtocolVersion, mcpProtocolVersion)
	req.Header.Set(headerMCPMethod, method)
	if p, ok := params.(map[string]any); ok && method == "tools/call" {
		req.Header.Set(headerMCPName, p["name"].(string))
	}
	for k, v := range headers {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}

	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("%s: HTTP %d: %s", method, w.Code, w.Body.String())
	}
	var reply rpcReply
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
		t.Fatalf("%s: bad JSON: %v", method, err)
	}
	return reply
}

func callToolText(t *testing.T, s *Server, name string, args map[string]any) callToolResult {
	t.Helper()
	reply := mcpCall(t, s, "tools/call", map[string]any{"name": name, "arguments": args}, nil)
	if reply.Error != nil {
		t.Fatalf("%s: %s", name, reply.Error.Message)
	}
	var result callToolResult
	if err := json.Unmarshal(reply.Result, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMCP_ToolsList(t *testing.T) {
	s := newTestServer(t)
	reply := mcpCall(t, s, "tools/list", nil, nil)
	if reply.Error != nil {
		t.Fatal(reply.Error.Message)
	}
	var result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(reply.Result, &result); err != nil {
		t.Fatal(err)
	}

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
	result := callToolText(t, s, "docs__search_docs", map[string]any{"query": "ttl unload"})
	if result.IsError || !strings.Contains(result.Content[0].Text, "ttl") {
		t.Errorf("search result: %+v", result)
	}
}

func TestMCP_ConfigSchema(t *testing.T) {
	s := newTestServer(t)
	result := callToolText(t, s, "docs__get_config_schema", map[string]any{"path": "models.*.ttl"})
	if result.IsError || !strings.Contains(result.Content[0].Text, "models.*.ttl") {
		t.Errorf("schema result: %+v", result)
	}
}

func TestMCP_CodeToolsReadEmbeddedSource(t *testing.T) {
	s := newTestServer(t)

	result := callToolText(t, s, "code__grep", map[string]any{"pattern": "func NewDocsProvider", "glob": "*.go"})
	if result.IsError || !strings.Contains(result.Content[0].Text, "internal/docagent/mcpprovider.go") {
		t.Errorf("grep result: %+v", result)
	}

	result = callToolText(t, s, "code__read", map[string]any{"path": "internal/docagent/mcpprovider.go"})
	if result.IsError || !strings.Contains(result.Content[0].Text, "github.com/mostlygeek/llama-swap/blob/") {
		t.Errorf("read result should link to GitHub: %.300s", result.Content[0].Text)
	}
}

func TestMCP_ToolErrorsAreResults(t *testing.T) {
	s := newTestServer(t)
	result := callToolText(t, s, "docs__get_doc", map[string]any{"id": "no/such/doc"})
	if !result.IsError {
		t.Errorf("unknown doc should be a tool error: %+v", result)
	}
}

func TestMCP_ProtocolErrors(t *testing.T) {
	s := newTestServer(t)

	reply := mcpCall(t, s, "tools/list", nil, map[string]string{headerMCPProtocolVersion: "2025-06-18"})
	if reply.Error == nil || reply.Error.Code != codeUnsupportedProtocolVersion {
		t.Errorf("old version: %+v", reply.Error)
	}

	reply = mcpCall(t, s, "tools/list", nil, map[string]string{headerMCPMethod: "tools/call"})
	if reply.Error == nil || reply.Error.Code != codeInvalidRequest {
		t.Errorf("method header mismatch: %+v", reply.Error)
	}

	reply = mcpCall(t, s, "tools/call", map[string]any{"name": "docs__list_docs"}, map[string]string{headerMCPName: ""})
	if reply.Error == nil || reply.Error.Code != codeInvalidRequest {
		t.Errorf("missing Mcp-Name: %+v", reply.Error)
	}

	reply = mcpCall(t, s, "tools/call", map[string]any{"name": "config__get_config"}, nil)
	if reply.Error == nil || !strings.Contains(reply.Error.Message, "unknown tool") {
		t.Errorf("unknown tool: %+v", reply.Error)
	}

	reply = mcpCall(t, s, "no/such/method", nil, nil)
	if reply.Error == nil || reply.Error.Code != codeMethodNotFound {
		t.Errorf("unknown method: %+v", reply.Error)
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
