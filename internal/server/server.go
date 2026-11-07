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
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mostlygeek/llama-swap/docbot"

	"github.com/mostlygeek/site-llama-swap.ai/internal/llamasrc"
	"github.com/mostlygeek/site-llama-swap.ai/web"
)

// Options configures a Server. Zero values take the defaults in agent.go.
type Options struct {
	// Version is the site's build version, shown in the MCP server info.
	Version string

	Model           string
	ModelRevision   string
	TransformersURL string

	// Logger receives the access log. Nil uses slog.Default().
	Logger *slog.Logger
}

// Server serves the whole site. It holds no per-request state.
type Server struct {
	mux *http.ServeMux
	bot *docbot.Handler
	log *slog.Logger

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
	if opts.TransformersURL == "" {
		opts.TransformersURL = DefaultTransformersURL
	}

	src, err := docbot.SourceFromArchive(llamasrc.Archive)
	if err != nil {
		return nil, fmt.Errorf("loading llama-swap source: %w", err)
	}
	bot, err := docbot.New(docbot.Options{Name: "llama-swap.ai", Version: opts.Version, Source: src})
	if err != nil {
		return nil, err
	}

	s := &Server{
		mux: http.NewServeMux(),
		bot: bot,
		log: opts.Logger,
	}
	if s.log == nil {
		s.log = slog.Default()
	}

	s.agent = AgentConfig{
		SystemPrompt:    bot.SystemPrompt(),
		Model:           opts.Model,
		Revision:        opts.ModelRevision,
		TransformersURL: opts.TransformersURL,
		MaxNewTokens:    2048,
		MaxIterations:   12,
		Suggestions:     suggestions,
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
	s.mux.HandleFunc("GET /help", s.page("help"))
	s.mux.Handle("/api/mcp", s.bot)
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("ok\n"))
	})

	static := http.FileServerFS(web.Static)
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", s.cacheStatic(static)))
	s.mux.Handle("GET /favicon.ico", s.cacheStatic(http.StripPrefix("/", http.FileServerFS(subFS(web.Static, "img")))))

	s.mux.HandleFunc("/", s.notFound)
}

// ServeHTTP adds the headers every response carries and logs the request.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("X-Frame-Options", "DENY")

	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	s.mux.ServeHTTP(rec, r)

	s.log.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status,
		"duration", time.Since(start).Round(time.Microsecond))
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
	// AgentJSON is the Help page's configuration, already JSON encoded.
	AgentJSON template.JS
}

func (s *Server) parseTemplates() error {
	funcs := template.FuncMap{
		// asset returns a static file's URL with the cache-busting version.
		"asset": func(name string) string {
			return "/static/" + strings.TrimPrefix(name, "/") + "?v=" + s.assetVersion
		},
	}

	s.pages = map[string]*template.Template{}
	for _, name := range []string{"home", "help", "404"} {
		t, err := template.New("base.html").Funcs(funcs).ParseFS(web.Templates,
			"templates/base.html", "templates/"+name+".html")
		if err != nil {
			return fmt.Errorf("parsing template %s: %w", name, err)
		}
		s.pages[name] = t
	}
	return nil
}

func (s *Server) render(w http.ResponseWriter, status int, name string) {
	data := pageData{Page: name}
	switch name {
	case "home":
		data.Title = "llama-swap: hot-swap local AI models"
	case "help":
		data.Title = "Help agent · llama-swap"
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
		s.render(w, http.StatusOK, name)
	}
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusNotFound, "404")
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
