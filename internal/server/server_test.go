package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/mostlygeek/site-llama-swap.ai/internal/llamasrc"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	s, err := New(Options{Version: "test", Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestServer_HomePage(t *testing.T) {
	s := newTestServer(t)
	w := get(t, s, "/")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"Run many local AI models", `href="/docs/chat"`, "/static/css/site.css?v=" + s.assetVersion} {
		if !strings.Contains(body, want) {
			t.Errorf("home page is missing %q", want)
		}
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", got)
	}
}

// agentConfigFromPage pulls the JSON the Help page embeds for help.js.
func agentConfigFromPage(t *testing.T, body string) AgentConfig {
	t.Helper()
	m := regexp.MustCompile(`(?s)<script type="application/json" id="agent-config">(.*?)</script>`).FindStringSubmatch(body)
	if m == nil {
		t.Fatal("help page has no agent-config script")
	}
	var cfg AgentConfig
	if err := json.Unmarshal([]byte(m[1]), &cfg); err != nil {
		t.Fatalf("agent-config is not valid JSON: %v\n%s", err, m[1])
	}
	return cfg
}

func TestServer_HelpPageAgentConfig(t *testing.T) {
	s := newTestServer(t)
	w := get(t, s, "/docs/chat")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	cfg := agentConfigFromPage(t, w.Body.String())
	if cfg.Model != DefaultModel || cfg.Revision != DefaultModelRevision {
		t.Errorf("model = %s@%s", cfg.Model, cfg.Revision)
	}
	if cfg.TransformersURL != DefaultTransformersURL {
		t.Errorf("transformersUrl = %s", cfg.TransformersURL)
	}
	if len(cfg.Suggestions) == 0 {
		t.Error("no suggestions")
	}

	// The prompt keeps the tools the site serves and drops the ones it does
	// not. The code__* tools are served only when the source was generated.
	want := []string{"`docs__search_docs`", "## How to answer"}
	unwanted := []string{"`config__get_config`", "`sys__now`"}
	if llamasrc.Archive() != nil {
		want = append(want, "`code__grep`")
	} else {
		unwanted = append(unwanted, "`code__grep`")
	}
	for _, want := range want {
		if !strings.Contains(cfg.SystemPrompt, want) {
			t.Errorf("system prompt is missing %s", want)
		}
	}
	for _, unwanted := range unwanted {
		if strings.Contains(cfg.SystemPrompt, unwanted) {
			t.Errorf("system prompt still lists %s", unwanted)
		}
	}
}

func TestServer_NotFound(t *testing.T) {
	s := newTestServer(t)
	w := get(t, s, "/no-such-page")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Page not found") {
		t.Error("404 page not rendered")
	}
}

func TestServer_StaticCaching(t *testing.T) {
	s := newTestServer(t)

	w := get(t, s, "/static/js/help.js?v="+s.assetVersion)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Errorf("versioned asset Cache-Control = %q", got)
	}
	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/javascript") {
		t.Errorf("Content-Type = %q", got)
	}

	w = get(t, s, "/static/js/help.js")
	if got := w.Header().Get("Cache-Control"); strings.Contains(got, "immutable") {
		t.Errorf("unversioned asset Cache-Control = %q", got)
	}

	if w := get(t, s, "/favicon.ico"); w.Code != http.StatusOK {
		t.Errorf("favicon status = %d", w.Code)
	}
}

func TestServer_Health(t *testing.T) {
	s := newTestServer(t)
	w := get(t, s, "/health")
	body, _ := io.ReadAll(w.Body)
	if w.Code != http.StatusOK || string(body) != "ok\n" {
		t.Fatalf("health = %d %q", w.Code, body)
	}
}
