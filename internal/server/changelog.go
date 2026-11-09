package server

import "net/http"

func (s *Server) changelogRoutes() {
	s.mux.HandleFunc("GET /releases", s.changelogIndex)
	s.mux.HandleFunc("GET /releases/{$}", s.changelogIndex)
	s.mux.HandleFunc("GET /releases/{version}", s.changelogRelease)

	// The old /changelog paths moved to /releases.
	s.mux.HandleFunc("GET /changelog", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases", http.StatusMovedPermanently)
	})
	s.mux.HandleFunc("GET /changelog/{rest...}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/"+r.PathValue("rest"), http.StatusMovedPermanently)
	})
}

func (s *Server) changelogData(title string) pageData {
	return pageData{
		Title:        title,
		Changelog:    s.changelog,
		HighlightCSS: "/docs/highlight.css?v=" + s.highlightVersion,
	}
}

func (s *Server) changelogIndex(w http.ResponseWriter, r *http.Request) {
	data := s.changelogData("Changelog · llama-swap")
	data.Description = "Release notes for every version of llama-swap, newest first."
	s.render(w, http.StatusOK, "changelog", data)
}

func (s *Server) changelogRelease(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")
	release, ok := s.changelog.Release(version)
	if !ok {
		s.notFound(w, r)
		return
	}
	if release.Version != version {
		// The version may be given without the leading "v".
		http.Redirect(w, r, "/releases/"+release.Version, http.StatusMovedPermanently)
		return
	}
	data := s.changelogData(release.Version + " · llama-swap changelog")
	data.Description = release.Summary
	data.Release = release
	s.render(w, http.StatusOK, "changelog_page", data)
}
