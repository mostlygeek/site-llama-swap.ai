package server

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestServer_FeaturesPages(t *testing.T) {
	s := newTestServer(t)

	w := get(t, s, "/features")
	if w.Code != http.StatusOK {
		t.Fatalf("/features status = %d", w.Code)
	}
	body := w.Body.String()
	first := s.features.Slides[0]
	if !strings.Contains(body, "<title>"+first.Title+" · llama-swap features</title>") {
		t.Error("/features does not show the first slide's title")
	}
	asked := map[string]bool{}
	for _, m := range regexp.MustCompile(`href="/docs/chat\?q=([^"]*)"`).FindAllStringSubmatch(body, -1) {
		q, err := url.QueryUnescape(m[1])
		if err != nil {
			t.Fatal(err)
		}
		asked[q] = true
	}
	// Every slide is on the page, so the script can switch without a reload.
	for _, slide := range s.features.Slides {
		if !strings.Contains(body, `data-slug="`+slide.Slug+`"`) {
			t.Errorf("slide %s is not on the page", slide.Slug)
		}
		if !strings.Contains(body, `<option value="`+slide.Slug+`"`) {
			t.Errorf("slide %s is not in the topic menu", slide.Slug)
		}
		if !asked[slide.Prompt] {
			t.Errorf("slide %s has no Ask the llama link for %q", slide.Slug, slide.Prompt)
		}
	}
	if !strings.Contains(body, `href="/features" aria-current="page"`) {
		t.Error("the Features nav link is not marked current")
	}

	last := s.features.Slides[len(s.features.Slides)-1]
	w = get(t, s, last.URL())
	if w.Code != http.StatusOK {
		t.Fatalf("%s status = %d", last.URL(), w.Code)
	}
	body = w.Body.String()
	if !strings.Contains(body, `id="slide-`+last.Slug+`" data-slug="`+last.Slug+`"`) {
		t.Fatal("the last slide is missing")
	}
	if strings.Contains(body, `id="slide-`+first.Slug+`" data-slug="`+first.Slug+`" data-title="`+first.Title) &&
		!strings.Contains(body, `aria-labelledby="slide-title-`+first.Slug+`" hidden`) {
		t.Error("the first slide is not hidden on the last slide's URL")
	}
	if !strings.Contains(body, `<option value="`+last.Slug+`" selected>`) {
		t.Error("the topic menu does not select the current slide")
	}

	if w := get(t, s, "/features/no-such-slide"); w.Code != http.StatusNotFound {
		t.Errorf("unknown slide status = %d", w.Code)
	}
}

// Each slide's Read more links must name real docs pages.
func TestServer_FeaturesDocLinks(t *testing.T) {
	s := newTestServer(t)
	for _, slide := range s.features.Slides {
		for _, id := range slide.Docs {
			if _, ok := s.docs.Page(id); !ok {
				t.Errorf("slide %s links to unknown doc %s", slide.Slug, id)
			}
		}
	}
}
