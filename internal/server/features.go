package server

import "net/http"

func (s *Server) featuresRoutes() {
	s.mux.HandleFunc("GET /features", s.featuresPage)
	s.mux.HandleFunc("GET /features/{$}", s.featuresPage)
	s.mux.HandleFunc("GET /features/{slug}", s.featuresPage)
}

// featuresPage serves the Features tour. Every URL gets every slide, so
// features.js can move between them without a reload; the URL picks which
// one is shown first.
func (s *Server) featuresPage(w http.ResponseWriter, r *http.Request) {
	slide := s.features.Slides[0]
	if slug := r.PathValue("slug"); slug != "" {
		var ok bool
		if slide, ok = s.features.Slide(slug); !ok {
			s.notFound(w, r)
			return
		}
	}
	s.render(w, http.StatusOK, "features", pageData{
		Title:        slide.Title + " · llama-swap features",
		Description:  "A tour of llama-swap's features, from a first config to routing several models at once.",
		Features:     s.features,
		Slide:        slide,
		HighlightCSS: "/docs/highlight.css?v=" + s.highlightVersion,
	})
}
