package featuresite

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mostlygeek/site-llama-swap.ai/web"
)

func TestLoad_SiteSlides(t *testing.T) {
	s, err := Load(web.Features)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s.Slides) < 10 {
		t.Fatalf("only %d slides", len(s.Slides))
	}

	// Slides go from the basics to the advanced settings.
	last := 0
	for i, slide := range s.Slides {
		if slide.Number != i+1 {
			t.Errorf("%s: number = %d, want %d", slide.Slug, slide.Number, i+1)
		}
		rank := 0
		for j, l := range Levels {
			if l == slide.Level {
				rank = j
			}
		}
		if rank < last {
			t.Errorf("%s: %s slide comes after a %s one", slide.Slug, slide.Level, Levels[last])
		}
		last = rank
		if !strings.Contains(string(slide.HTML), "<pre") {
			t.Errorf("%s: no config or command example", slide.Slug)
		}
	}
	if s.Slides[0].Prev != nil || s.Slides[len(s.Slides)-1].Next != nil {
		t.Error("the first slide has a previous slide or the last has a next one")
	}
	if s.Slides[1].Prev != s.Slides[0] || s.Slides[0].Next != s.Slides[1] {
		t.Error("prev and next are not linked")
	}
	if _, ok := s.Slide("web-ui"); !ok {
		t.Error("no web-ui slide")
	}

	// Screenshots without a full URL are files under web/static.
	for _, slide := range s.Slides {
		for _, shot := range slide.Screenshots {
			if strings.HasPrefix(shot.Src, "https://") {
				continue
			}
			if _, err := fs.Stat(web.Static, shot.Src); err != nil {
				t.Errorf("%s: screenshot %s: %v", slide.Slug, shot.Src, err)
			}
		}
	}
}

func TestLoad_Errors(t *testing.T) {
	good := "---\ntitle: T\nlevel: Basics\nprompt: Q?\n---\nBody\n"
	tests := map[string]fstest.MapFS{
		"bad name":       {"intro.md": {Data: []byte(good)}},
		"no prompt":      {"01-a.md": {Data: []byte("---\ntitle: T\nlevel: Basics\n---\nBody\n")}},
		"bad level":      {"01-a.md": {Data: []byte("---\ntitle: T\nlevel: Expert\nprompt: Q?\n---\nBody\n")}},
		"no frontmatter": {"01-a.md": {Data: []byte("Body\n")}},
		"duplicate":      {"01-a.md": {Data: []byte(good)}, "02-a.md": {Data: []byte(good)}},
		"empty":          {},
	}
	for name, fsys := range tests {
		if _, err := Load(fsys); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestLoad_EscapesRawHTML(t *testing.T) {
	fsys := fstest.MapFS{"01-a.md": {Data: []byte("---\ntitle: T\nlevel: Basics\nprompt: Q?\n---\n<script>alert(1)</script>\n")}}
	s, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(s.Slides[0].HTML), "<script>") {
		t.Errorf("raw HTML was not escaped: %s", s.Slides[0].HTML)
	}
}
