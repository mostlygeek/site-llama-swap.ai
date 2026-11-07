package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"

	"github.com/mostlygeek/site-llama-swap.ai/internal/docsite"
)

func (s *Server) docsRoutes() {
	s.mux.HandleFunc("GET /docs", s.docsIndex)
	s.mux.HandleFunc("GET /docs/{$}", s.docsIndex)
	s.mux.HandleFunc("GET /docs/search", s.docsSearch)
	s.mux.HandleFunc("GET /docs/highlight.css", s.docsHighlightCSS)
	s.mux.HandleFunc("GET /docs/"+docsite.ConfigID, s.docsConfig)
	s.mux.HandleFunc("GET /docs/{id...}", s.docsPage)
}

func (s *Server) docsData(title string) pageData {
	return pageData{
		Title:        title,
		Docs:         s.docs,
		HighlightCSS: "/docs/highlight.css?v=" + s.highlightVersion,
	}
}

func (s *Server) docsIndex(w http.ResponseWriter, r *http.Request) {
	data := s.docsData("Documentation · llama-swap")
	data.Description = "Guides and the configuration reference for llama-swap."
	s.render(w, http.StatusOK, "docs_index", data)
}

func (s *Server) docsPage(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(r.PathValue("id"), "/")
	page, ok := s.docs.Page(id)
	if !ok {
		// Agents and people sometimes write the file name.
		if page, ok = s.docs.Page(strings.TrimSuffix(id, ".md")); ok {
			http.Redirect(w, r, page.URL(), http.StatusMovedPermanently)
			return
		}
		s.notFound(w, r)
		return
	}
	data := s.docsData(page.Title + " · llama-swap docs")
	data.Description = page.Summary
	data.Doc = page
	s.render(w, http.StatusOK, "docs_page", data)
}

func (s *Server) docsConfig(w http.ResponseWriter, r *http.Request) {
	page, _ := s.docs.Page(docsite.ConfigID)
	data := s.docsData(page.Title + " · llama-swap docs")
	data.Description = page.Summary
	data.Doc = page
	s.render(w, http.StatusOK, "docs_config", data)
}

func (s *Server) docsHighlightCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	if r.URL.Query().Get("v") == s.highlightVersion {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	w.Write([]byte(s.docs.HighlightCSS))
}

// SearchResult is one hit on the docs search page.
type SearchResult struct {
	Title   string
	URL     string
	ID      string
	Snippet string
}

// maxSearchQuery bounds the query a visitor can send.
const maxSearchQuery = 200

func (s *Server) docsSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > maxSearchQuery {
		q = q[:maxSearchQuery]
	}
	data := s.docsData("Search the docs · llama-swap")
	data.Query = q
	if q != "" {
		data.Title = q + " · Search the docs · llama-swap"
		results, err := s.searchDocs(r, q)
		if err != nil {
			s.log.Error("docs search", "query", q, "error", err)
			data.SearchMessage = "Search is not working right now."
		} else {
			data.Results = results
			if len(results) == 0 {
				data.SearchMessage = "Nothing matched. Try fewer or different words."
			}
		}
	}
	s.render(w, http.StatusOK, "docs_search", data)
}

// searchHit matches one result in the text docs__search_docs returns:
//
//  1. guides/model-runtime/ttl-and-unloading (line 17) — Automatic model unloading with ttl
//     ```
//     snippet
//     ```
var searchHit = regexp.MustCompile("(?m)^\\d+\\. (\\S+) \\(line \\d+\\) — (.+)\\n```\\n((?s:.*?))\\n```")

// searchDocs runs the Help agent's own docs__search_docs tool, by calling
// docbot's MCP handler in memory, so the search page and the agent find the
// same things.
func (s *Server) searchDocs(r *http.Request, q string) ([]SearchResult, error) {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "docs__search_docs",
			"arguments": map[string]any{"query": q, "max_results": 15},
		},
	})
	req := httptest.NewRequestWithContext(r.Context(), http.MethodPost, "/api/mcp", strings.NewReader(string(body)))
	req.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	req.Header.Set("Mcp-Method", "tools/call")
	req.Header.Set("Mcp-Name", "docs__search_docs")
	rec := httptest.NewRecorder()
	s.bot.ServeHTTP(rec, req)

	var reply struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		return nil, fmt.Errorf("decoding search reply: %w", err)
	}
	if reply.Error != nil {
		return nil, fmt.Errorf("search: %s", reply.Error.Message)
	}
	if reply.Result.IsError || len(reply.Result.Content) == 0 {
		return nil, nil
	}

	text := reply.Result.Content[0].Text
	var results []SearchResult
	seen := map[string]bool{}
	for _, m := range searchHit.FindAllStringSubmatch(text, -1) {
		id, title, snippet := m[1], strings.TrimSpace(m[2]), m[3]
		url := docsite.URL(id)
		if page, ok := s.docs.Page(id); ok {
			title = page.Title
		}
		// Several hits can land in one document; show its first.
		if seen[url] {
			continue
		}
		seen[url] = true
		results = append(results, SearchResult{Title: title, URL: url, ID: id, Snippet: snippet})
	}
	if len(results) == 0 && !strings.HasPrefix(text, "No matches") {
		return nil, fmt.Errorf("could not read the search results: %.200q", text)
	}
	return results, nil
}
