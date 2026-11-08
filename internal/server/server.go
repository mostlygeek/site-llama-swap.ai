// Package server is the llama-swap.ai website: a landing page, the Help page
// that runs llama-swap's Docs Agent in the browser, and the MCP endpoint that
// agent's tools call.
package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mostlygeek/llama-swap/docbot"
	"github.com/mostlygeek/llama-swap/docs"

	"github.com/mostlygeek/site-llama-swap.ai/internal/changelogsite"
	"github.com/mostlygeek/site-llama-swap.ai/internal/docsite"
	"github.com/mostlygeek/site-llama-swap.ai/internal/llamasrc"
	"github.com/mostlygeek/site-llama-swap.ai/web"
)

// Options configures a Server. Zero values take the defaults in agent.go.
type Options struct {
	// Version is the site's build version, shown in the MCP server info.
	Version string

	Model         string
	ModelRevision string
	ModelFile     string

	// Logger receives the site log. Nil uses slog.Default().
	Logger *slog.Logger
}

// Server serves the whole site. It holds no per-request state.
type Server struct {
	mux  *http.ServeMux
	bot  *docbot.Handler
	docs *docsite.Site
	// changelog renders llama-swap's release notes.
	changelog *changelogsite.Site
	log       *slog.Logger

	// highlightVersion versions the code colours stylesheet's URL.
	highlightVersion string

	pages map[string]*template.Template
	agent AgentConfig

	// assetVersion is a hash of the static files, appended to their URLs so
	// they can be cached for a long time and still change on deploy.
	assetVersion string
}

// New builds the documentation index, the tool registry and the page
// templates.
func New(opts Options) (*Server, error) {
	if opts.Model == "" {
		opts.Model = DefaultModel
	}
	if opts.ModelRevision == "" {
		opts.ModelRevision = DefaultModelRevision
	}
	if opts.ModelFile == "" {
		opts.ModelFile = DefaultModelFile
	}

	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}

	// Without the source archive the site still works, serving the docs__*
	// tools only; docbot then leaves the code rules out of the prompt.
	var src fs.FS
	if archive := llamasrc.Archive(); archive != nil {
		var err error
		if src, err = docbot.SourceFromArchive(archive); err != nil {
			return nil, fmt.Errorf("loading llama-swap source: %w", err)
		}
	} else {
		log.Warn("llama-swap source is not embedded, so the code__* tools are off; build with make")
	}
	bot, err := docbot.New(docbot.Options{Name: "llama-swap.ai", Version: opts.Version, Source: src})
	if err != nil {
		return nil, err
	}

	site, err := docsite.Load(docs.Files)
	if err != nil {
		return nil, fmt.Errorf("rendering the llama-swap docs: %w", err)
	}

	cl, err := changelogsite.Load()
	if err != nil {
		return nil, fmt.Errorf("rendering the llama-swap changelog: %w", err)
	}

	s := &Server{
		mux:       http.NewServeMux(),
		bot:       bot,
		docs:      site,
		changelog: cl,
		log:       log,
	}
	s.highlightVersion = fmt.Sprintf("%x", sha256.Sum256([]byte(site.HighlightCSS)))[:12]

	s.agent = AgentConfig{
		SystemPrompt:  bot.SystemPrompt(),
		Model:         opts.Model,
		Revision:      opts.ModelRevision,
		ModelFile:     opts.ModelFile,
		ContextSize:   32768,
		MaxNewTokens:  2048,
		MaxIterations: 12,
		Suggestions:   suggestions,
		DocLinks:      site.Links(),
	}

	if s.assetVersion, err = hashFS(web.Static); err != nil {
		return nil, fmt.Errorf("hashing static files: %w", err)
	}
	if err := s.parseTemplates(); err != nil {
		return nil, err
	}

	s.routes()
	return s, nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /{$}", s.page("home"))
	// The chat lives under the docs now; keep old links working.
	s.mux.HandleFunc("GET /help", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/chat", http.StatusMovedPermanently)
	})
	s.mux.Handle("/api/mcp", s.bot)
	s.docsRoutes()
	s.changelogRoutes()
	s.mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("ok\n"))
	})

	static := http.FileServerFS(web.Static)
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", s.cacheStatic(static)))
	s.mux.Handle("GET /favicon.ico", s.cacheStatic(http.StripPrefix("/", http.FileServerFS(subFS(web.Static, "img")))))

	s.mux.HandleFunc("/", s.notFound)
}

// ServeHTTP adds the headers every response carries. Every request to the
// /api/mcp endpoint is logged (see mcpInspect); other requests are silent.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("X-Frame-Options", "DENY")
	// Cross-origin isolation gives the chat page SharedArrayBuffer, which
	// wllama needs to run the model on several threads. "credentialless"
	// still lets the page load cross-origin files, without cookies.
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Cross-Origin-Embedder-Policy", "credentialless")

	method, tool, query := mcpInspect(r)

	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	s.mux.ServeHTTP(rec, r)

	if r.URL.Path == "/api/mcp" {
		args := []any{"http_method", r.Method}
		if method != "" {
			args = append(args, "method", method)
			if tool != "" {
				args = append(args, "tool", tool)
			}
			if query != "" {
				args = append(args, "query", query)
			}
		}
		args = append(args, "status", rec.status,
			"duration", time.Since(start).Round(time.Microsecond))
		s.log.Info("mcp", args...)
	}
}

// mcpInspect inspects a request to the /api/mcp endpoint and returns its
// JSON-RPC method, the tool name of a tools/call, and the query argument
// when present. The body is buffered so the MCP handler can still read
// it.
func mcpInspect(r *http.Request) (string, string, string) {
	if r.URL.Path != "/api/mcp" || r.Method != http.MethodPost {
		return "", "", ""
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", "", ""
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	var req struct {
		Method string `json:"method"`
		Params struct {
			Name      string `json:"name"`
			Arguments struct {
				Query string `json:"query"`
			} `json:"arguments"`
		} `json:"params"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", "", ""
	}
	return req.Method, req.Params.Name, req.Params.Arguments.Query
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// cacheStatic lets browsers keep a static file for a year when the URL
// carries the current asset version, and revalidate otherwise.
func (s *Server) cacheStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") == s.assetVersion {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=300")
		}
		next.ServeHTTP(w, r)
	})
}

// pageData is what every template receives.
type pageData struct {
	Page  string
	Title string
	// Description overrides the default meta description.
	Description string
	// AgentJSON is the Help page's configuration, already JSON encoded.
	AgentJSON template.JS

	// The docs pages.
	Docs          *docsite.Site
	Doc           *docsite.Page
	HighlightCSS  string
	Query         string
	Results       []SearchResult
	SearchMessage string

	// The changelog pages.
	Changelog *changelogsite.Site
	Release   *changelogsite.Release
}

func (s *Server) parseTemplates() error {
	funcs := template.FuncMap{
		// asset returns a static file's URL with the cache-busting version.
		"asset": func(name string) string {
			return "/static/" + strings.TrimPrefix(name, "/") + "?v=" + s.assetVersion
		},
		"hasPrefix": strings.HasPrefix,
	}

	s.pages = map[string]*template.Template{}
	for _, name := range []string{"home", "help", "404", "docs_index", "docs_page", "docs_config", "docs_search", "changelog", "changelog_page"} {
		files := []string{"templates/base.html", "templates/" + name + ".html"}
		switch {
		case strings.HasPrefix(name, "docs_"):
			files = append(files, "templates/docs_nav.html")
		case name == "changelog_page":
			files = append(files, "templates/changelog_nav.html")
		}
		t, err := template.New("base.html").Funcs(funcs).ParseFS(web.Templates, files...)
		if err != nil {
			return fmt.Errorf("parsing template %s: %w", name, err)
		}
		s.pages[name] = t
	}
	return nil
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data pageData) {
	data.Page = name
	switch name {
	case "home":
		data.Title = "llama-swap: hot-swap local AI models"
	case "help":
		data.Title = "Chat with the docs · llama-swap"
		agentJSON, err := json.Marshal(s.agent)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		data.AgentJSON = template.JS(agentJSON)
	case "404":
		data.Title = "Not found · llama-swap"
	}

	// Render to a buffer first so a template error becomes a clean 500
	// rather than half a page.
	var buf bytes.Buffer
	if err := s.pages[name].Execute(&buf, data); err != nil {
		s.log.Error("rendering page", "page", name, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

func (s *Server) page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, http.StatusOK, name, pageData{})
	}
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusNotFound, "404", pageData{})
}

// hashFS returns a short hash of every file in fsys.
func hashFS(fsys fs.FS) (string, error) {
	h := sha256.New()
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", name, len(data))
		h.Write(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}

func subFS(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
