package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestDocs_Pages(t *testing.T) {
	s := newTestServer(t)
	cases := []struct {
		path   string
		status int
		want   string
	}{
		{"/docs", 200, "Automatic model unloading with ttl"},
		{"/docs/guides/model-runtime/ttl-and-unloading", 200, "On this page"},
		{"/docs/reference/config", 200, `id="models"`},
		{"/docs/reference/readme", 200, "<h1>"},
		{"/docs/no/such/page", 404, "Page not found"},
	}
	for _, c := range cases {
		w := get(t, s, c.path)
		if w.Code != c.status || !strings.Contains(w.Body.String(), c.want) {
			t.Errorf("GET %s = %d, want %d containing %q", c.path, w.Code, c.status, c.want)
		}
	}

	w := get(t, s, "/docs/guides/model-runtime/ttl-and-unloading.md")
	if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != "/docs/guides/model-runtime/ttl-and-unloading" {
		t.Errorf(".md redirect = %d %s", w.Code, w.Header().Get("Location"))
	}

	w = get(t, s, "/docs/highlight.css")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/css") {
		t.Errorf("highlight.css = %d %s", w.Code, w.Header().Get("Content-Type"))
	}

	if !strings.Contains(get(t, s, "/").Body.String(), `href="/docs"`) {
		t.Error("header does not link to /docs")
	}
}

// Search runs docbot's docs__search_docs tool and reads its text output, so
// this fails if llama-swap changes that format.
func TestDocs_Search(t *testing.T) {
	s := newTestServer(t)
	body := get(t, s, "/docs/search?q=ttl+unload").Body.String()
	if !strings.Contains(body, `href="/docs/guides/model-runtime/ttl-and-unloading"`) {
		t.Errorf("search did not find the ttl guide:\n%.2000s", body)
	}
	if !strings.Contains(get(t, s, "/docs/search?q=zzqqxxnothing").Body.String(), "Nothing matched") {
		t.Error("empty search should say nothing matched")
	}
}

func TestDocs_AgentConfigLinksDocs(t *testing.T) {
	s := newTestServer(t)
	cfg := agentConfigFromPage(t, get(t, s, "/docs/chat").Body.String())
	if cfg.DocLinks["guides/model-runtime/ttl-and-unloading"] != "/docs/guides/model-runtime/ttl-and-unloading" {
		t.Errorf("docLinks missing the ttl guide")
	}
	if cfg.DocLinks["reference/config/models"] != "/docs/reference/config#models" {
		t.Errorf("docLinks missing config sections")
	}
}

// The chat moved under the docs; /help still works.
func TestDocs_ChatPageAndOldHelpLink(t *testing.T) {
	s := newTestServer(t)
	w := get(t, s, "/help")
	if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != "/docs/chat" {
		t.Errorf("/help = %d %s, want a redirect to /docs/chat", w.Code, w.Header().Get("Location"))
	}
	body := get(t, s, "/docs/chat").Body.String()
	if !strings.Contains(body, "Chat with the docs") || !strings.Contains(body, `id="agent-config"`) {
		t.Error("/docs/chat is not the chat page")
	}
	index := get(t, s, "/docs").Body.String()
	if !strings.Contains(index, `class="chat-tile" href="/docs/chat"`) || !strings.Contains(index, "runs in your browser") || !strings.Contains(index, "Ask the llama") {
		t.Error("the docs index has no chat tile")
	}
	if strings.Contains(get(t, s, "/").Body.String(), ">Help agent<") {
		t.Error("the header still has a Help agent link")
	}
}
