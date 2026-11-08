// Package featuresite renders the Features tour: a slide deck that
// introduces llama-swap one feature at a time, from the basics to the
// advanced settings.
//
// Each slide is a Markdown file in web/features, named NN-slug.md so the
// file name sets the order and the URL. A YAML front matter block gives the
// slide's title, its level, the question its "Ask the llama" button puts in
// the chat, the docs it links to, and optional screenshots. Everything is
// rendered once, when the site starts.
package featuresite

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"gopkg.in/yaml.v3"
)

// Slide is one rendered slide.
type Slide struct {
	// Slug is the slide's URL segment, from its file name.
	Slug  string
	Title string
	// Level is "Basics", "Intermediate" or "Advanced".
	Level string
	// Summary is one line under the title.
	Summary string
	// Prompt is the question the "Ask the llama" button fills in.
	Prompt string
	// Docs are knowledge base ids of related docs pages.
	Docs        []string
	Screenshots []Screenshot
	// Tabs are shown after the body, one panel at a time.
	Tabs []Tab
	HTML template.HTML
	// Number counts from 1.
	Number int
	// Prev and Next are the neighbouring slides, nil at either end.
	Prev, Next *Slide
}

// Tab is one panel of a slide's tabbed section.
type Tab struct {
	Title string
	HTML  template.HTML
}

// Screenshot is an image on a slide.
type Screenshot struct {
	Title   string `yaml:"title"`
	Src     string `yaml:"src"`
	Alt     string `yaml:"alt"`
	Caption string `yaml:"caption"`
}

// URL is the slide's path on the site.
func (s *Slide) URL() string { return "/features/" + s.Slug }

// Levels lists the allowed levels in order.
var Levels = []string{"Basics", "Intermediate", "Advanced"}

// Site holds every slide in order.
type Site struct {
	Slides []*Slide
	bySlug map[string]*Slide
}

type frontmatter struct {
	Title       string       `yaml:"title"`
	Level       string       `yaml:"level"`
	Summary     string       `yaml:"summary"`
	Prompt      string       `yaml:"prompt"`
	Docs        []string     `yaml:"docs"`
	Screenshots []Screenshot `yaml:"screenshots"`
	Tabs        []struct {
		Title string `yaml:"title"`
		Body  string `yaml:"body"`
	} `yaml:"tabs"`
}

var fileName = regexp.MustCompile(`^(\d+)-([a-z0-9-]+)\.md$`)

// Load renders every slide in fsys, a directory of NN-slug.md files.
func Load(fsys fs.FS) (*Site, error) {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			highlighting.NewHighlighting(
				highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
			),
		),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)

	names, err := fs.Glob(fsys, "*.md")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)

	s := &Site{bySlug: map[string]*Slide{}}
	for _, name := range names {
		m := fileName.FindStringSubmatch(path.Base(name))
		if m == nil {
			return nil, fmt.Errorf("%s: slide files are named NN-slug.md", name)
		}
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		slide, err := render(md, m[2], raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if _, dup := s.bySlug[slide.Slug]; dup {
			return nil, fmt.Errorf("%s: slug %q is used twice", name, slide.Slug)
		}
		slide.Number = len(s.Slides) + 1
		s.bySlug[slide.Slug] = slide
		s.Slides = append(s.Slides, slide)
	}
	if len(s.Slides) == 0 {
		return nil, fmt.Errorf("no slides found")
	}
	for i, slide := range s.Slides {
		if i > 0 {
			slide.Prev = s.Slides[i-1]
		}
		if i+1 < len(s.Slides) {
			slide.Next = s.Slides[i+1]
		}
	}
	return s, nil
}

// Levels lists the levels in order, for the topic menu.
func (s *Site) Levels() []string { return Levels }

// Slide returns the slide with the given slug.
func (s *Site) Slide(slug string) (*Slide, bool) {
	slide, ok := s.bySlug[slug]
	return slide, ok
}

func render(md goldmark.Markdown, slug string, raw []byte) (*Slide, error) {
	fm, body, err := splitFrontmatter(raw)
	if err != nil {
		return nil, err
	}
	if fm.Title == "" || fm.Prompt == "" {
		return nil, fmt.Errorf("front matter needs a title and a prompt")
	}
	if !validLevel(fm.Level) {
		return nil, fmt.Errorf("level %q is not one of %v", fm.Level, Levels)
	}
	for _, shot := range fm.Screenshots {
		if shot.Src == "" || shot.Alt == "" {
			return nil, fmt.Errorf("screenshot %q needs a src and alt text", shot.Title)
		}
	}
	var buf bytes.Buffer
	if err := md.Convert(body, &buf); err != nil {
		return nil, err
	}
	var tabs []Tab
	for _, t := range fm.Tabs {
		if t.Title == "" || strings.TrimSpace(t.Body) == "" {
			return nil, fmt.Errorf("tab %q needs a title and a body", t.Title)
		}
		var tb bytes.Buffer
		if err := md.Convert([]byte(t.Body), &tb); err != nil {
			return nil, err
		}
		tabs = append(tabs, Tab{Title: t.Title, HTML: template.HTML(tb.String())})
	}
	return &Slide{
		Slug:        slug,
		Title:       fm.Title,
		Level:       fm.Level,
		Summary:     fm.Summary,
		Prompt:      strings.TrimSpace(fm.Prompt),
		Docs:        fm.Docs,
		Screenshots: fm.Screenshots,
		Tabs:        tabs,
		HTML:        template.HTML(buf.String()),
	}, nil
}

func validLevel(level string) bool {
	for _, l := range Levels {
		if l == level {
			return true
		}
	}
	return false
}

func splitFrontmatter(raw []byte) (frontmatter, []byte, error) {
	var fm frontmatter
	text := string(raw)
	if !strings.HasPrefix(text, "---\n") {
		return fm, nil, fmt.Errorf("missing front matter")
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return fm, nil, fmt.Errorf("front matter is not closed")
	}
	if err := yaml.Unmarshal([]byte(text[4:4+end]), &fm); err != nil {
		return fm, nil, fmt.Errorf("front matter: %w", err)
	}
	return fm, []byte(text[4+end+5:]), nil
}
