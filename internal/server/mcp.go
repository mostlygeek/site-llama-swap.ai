package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mostlygeek/site-llama-swap.ai/internal/llamaswap/mcptools"
)

// The MCP endpoint here follows llama-swap's own /api/mcp
// (internal/server/apimcp.go in llama-swap) so the Help agent sees the same
// protocol, tool names and results it was trained against. Keep the two in
// step when llama-swap changes its endpoint.

// mcpProtocolVersion is the only MCP revision this endpoint speaks. 2026-07-28
// removed the initialize handshake and the session header, so every request
// stands alone and the endpoint needs no session state.
const mcpProtocolVersion = "2026-07-28"

const (
	headerMCPProtocolVersion = "Mcp-Protocol-Version"
	headerMCPMethod          = "Mcp-Method"
	headerMCPName            = "Mcp-Name"
)

// JSON-RPC 2.0 error codes, plus the MCP code for an unsupported version.
const (
	codeParseError                 = -32700
	codeInvalidRequest             = -32600
	codeMethodNotFound             = -32601
	codeInvalidParams              = -32602
	codeInternalError              = -32603
	codeUnsupportedProtocolVersion = -32022
)

// maxMCPBodyBytes bounds the request body. Tool arguments are a small JSON
// object.
const maxMCPBodyBytes = 64 * 1024

// toolListTTLMs tells clients how long to cache tools/list. The tools only
// change when the binary is rebuilt.
const toolListTTLMs = 3_600_000

const metaKeyServerInfo = "io.modelcontextprotocol/serverInfo"

const mcpInstructions = "Tools are namespaced by provider. The docs__* tools answer questions about " +
	"llama-swap's configuration: call docs__list_docs for an index, docs__search_docs to find " +
	"something by keyword, docs__get_doc to read a document in full, and docs__get_config_schema to " +
	"check what a configuration key accepts. Document ids starting with 'reference/config/' return " +
	"sections of config.example.yaml verbatim. " +
	"The code__* tools search and read llama-swap's source code, to check the documentation against " +
	"the implementation: code__grep for a regex search, code__read to read a file, code__find to find " +
	"files by glob, and code__ls to list a directory."

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type mcpCapabilities struct {
	Tools *struct{} `json:"tools,omitempty"`
}

type mcpImplementation struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type discoverResult struct {
	SupportedVersions []string        `json:"supportedVersions"`
	Capabilities      mcpCapabilities `json:"capabilities"`
	Instructions      string          `json:"instructions,omitempty"`
	TTLMs             int             `json:"ttlMs"`
	CacheScope        string          `json:"cacheScope"`
	Meta              map[string]any  `json:"_meta,omitempty"`
}

type listToolsResult struct {
	Tools      []mcptools.Tool `json:"tools"`
	NextCursor string          `json:"nextCursor,omitempty"`
	TTLMs      int             `json:"ttlMs"`
	CacheScope string          `json:"cacheScope"`
}

type callToolParams struct {
	Name      string                     `json:"name"`
	Arguments map[string]json.RawMessage `json:"arguments"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type callToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// handleMCP serves the documentation and source tools as a stateless MCP
// server. Like llama-swap's endpoint it never streams, so it answers
// application/json whatever the Accept header says.
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxMCPBodyBytes+1))
	if err != nil {
		writeRPCError(w, nullID(), codeParseError, "could not read request body", nil)
		return
	}
	if len(body) > maxMCPBodyBytes {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	if strings.HasPrefix(strings.TrimSpace(string(body)), "[") {
		writeRPCError(w, nullID(), codeInvalidRequest, "JSON-RPC batching is not supported", nil)
		return
	}

	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil || req.Method == "" {
		writeRPCError(w, nullID(), codeParseError, "request is not a JSON-RPC message", nil)
		return
	}

	if version := r.Header.Get(headerMCPProtocolVersion); version != mcpProtocolVersion {
		data, _ := json.Marshal(map[string]any{
			"supported": []string{mcpProtocolVersion},
			"requested": version,
		})
		writeRPCError(w, req.ID, codeUnsupportedProtocolVersion,
			fmt.Sprintf("unsupported protocol version; this server speaks MCP %s only", mcpProtocolVersion), data)
		return
	}

	if header := r.Header.Get(headerMCPMethod); header == "" {
		writeRPCError(w, req.ID, codeInvalidRequest, fmt.Sprintf("missing required %s header", headerMCPMethod), nil)
		return
	} else if header != req.Method {
		writeRPCError(w, req.ID, codeInvalidRequest,
			fmt.Sprintf("header mismatch: %s is %q but the request method is %q", headerMCPMethod, header, req.Method), nil)
		return
	}

	// A notification carries no id and expects no response body.
	if isNotification(req.ID) {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	switch req.Method {
	case "server/discover":
		writeRPCResult(w, req.ID, discoverResult{
			SupportedVersions: []string{mcpProtocolVersion},
			Capabilities:      mcpCapabilities{Tools: &struct{}{}},
			Instructions:      mcpInstructions,
			TTLMs:             toolListTTLMs,
			CacheScope:        "public",
			Meta: map[string]any{
				metaKeyServerInfo: mcpImplementation{Name: "llama-swap.ai", Version: s.version},
			},
		})
	case "tools/list":
		s.mcpToolsList(w, r, req)
	case "tools/call":
		s.mcpToolsCall(w, r, req)
	case "ping":
		writeRPCResult(w, req.ID, struct{}{})
	default:
		writeRPCError(w, req.ID, codeMethodNotFound, fmt.Sprintf("unknown method %q", req.Method), nil)
	}
}

func (s *Server) mcpToolsList(w http.ResponseWriter, r *http.Request, req rpcRequest) {
	var params struct {
		Cursor string `json:"cursor"`
	}
	if len(req.Params) > 0 {
		_ = json.Unmarshal(req.Params, &params)
	}

	tools, nextCursor, err := s.tools.Tools(r.Context(), params.Cursor, mcptools.DefaultPageSize)
	if err != nil {
		if errors.Is(err, mcptools.ErrInvalidCursor) {
			writeRPCError(w, req.ID, codeInvalidParams, "invalid cursor", nil)
			return
		}
		writeRPCError(w, req.ID, codeInternalError, fmt.Sprintf("listing tools: %v", err), nil)
		return
	}
	if tools == nil {
		tools = []mcptools.Tool{}
	}

	writeRPCResult(w, req.ID, listToolsResult{
		Tools:      tools,
		NextCursor: nextCursor,
		TTLMs:      toolListTTLMs,
		CacheScope: "public",
	})
}

func (s *Server) mcpToolsCall(w http.ResponseWriter, r *http.Request, req rpcRequest) {
	var params callToolParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			writeRPCError(w, req.ID, codeInvalidRequest, "params must be an object with a name", nil)
			return
		}
	}
	if params.Name == "" {
		writeRPCError(w, req.ID, codeInvalidRequest, "params.name is required", nil)
		return
	}

	if header := r.Header.Get(headerMCPName); header == "" {
		writeRPCError(w, req.ID, codeInvalidRequest,
			fmt.Sprintf("missing required %s header for tools/call", headerMCPName), nil)
		return
	} else if header != params.Name {
		writeRPCError(w, req.ID, codeInvalidRequest,
			fmt.Sprintf("header mismatch: %s is %q but params.name is %q", headerMCPName, header, params.Name), nil)
		return
	}

	result, found, err := s.tools.Call(r.Context(), params.Name, params.Arguments)
	if !found {
		writeRPCError(w, req.ID, codeInvalidRequest, fmt.Sprintf("unknown tool %q", params.Name), nil)
		return
	}
	if err != nil {
		writeRPCError(w, req.ID, codeInternalError, err.Error(), nil)
		return
	}

	writeRPCResult(w, req.ID, callToolResult{
		Content: []mcpContent{{Type: "text", Text: result.Content}},
		IsError: result.IsError,
	})
}

func isNotification(id json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(id))
	return trimmed == "" || trimmed == "null"
}

func nullID() json.RawMessage { return json.RawMessage("null") }

// JSON-RPC responses, errors included, are HTTP 200. Only transport failures
// use HTTP status codes.
func writeRPCResult(w http.ResponseWriter, id json.RawMessage, result any) {
	writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: normalizeID(id), Result: result})
}

func writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, message string, data json.RawMessage) {
	writeRPC(w, rpcResponse{
		JSONRPC: "2.0",
		ID:      normalizeID(id),
		Error:   &rpcError{Code: code, Message: message, Data: data},
	})
}

func writeRPC(w http.ResponseWriter, response rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(headerMCPProtocolVersion, mcpProtocolVersion)
	_ = json.NewEncoder(w).Encode(response)
}

func normalizeID(id json.RawMessage) json.RawMessage {
	if len(id) == 0 {
		return nullID()
	}
	return id
}
