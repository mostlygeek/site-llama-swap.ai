// Package changelogsite renders llama-swap's release notes as HTML pages.
//
// The Markdown comes from llama-swap's changelog package, one file per
// release, at the llama-swap version the site is built with. Everything is
// rendered once, when the site starts: the notes only change when the
// binary does.
package changelogsite

import (
	"bytes"
	"fmt"
	"html/template"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/mostlygeek/llama-swap/changelog"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/mostlygeek/site-llama-swap.ai/internal/docsite"
)

// repoBlob is where links to files outside the docs point.
const repoBlob = "https://github.com/mostlygeek/llama-swap/blob/main/"

// Release is one rendered release note.
type Release struct {
	Version string
	// Title is the note's first heading, for example "v271 (Nov 8, 2026)".
	Title string
	// Date is the date in the title, for example "Nov 8, 2026". Empty for
	// notes without one.
	Date string
	// Summary is the note's first paragraph, for listings.
	Summary string
	HTML    template.HTML
	// TOC lists the note's second and third level headings.
	TOC []docsite.Heading
	// Source is the file's path in the llama-swap repository.
	Source string
}

// URL is the release's path on the site.
func (r *Release) URL() string { return "/releases/" + r.Version }

// SourceURL links to the release's Markdown on GitHub.
func (r *Release) SourceURL() string { return repoBlob + r.Source }

// Site holds every rendered release, newest first.
type Site struct {
	Releases []*Release
	byNum    map[string]*Release
}

// Load renders every release note in the changelog package.
func Load() (*Site, error) {
	s := &Site{byNum: map[string]*Release{}}
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			highlighting.NewHighlighting(
				highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithASTTransformers(util.Prioritized(linkRewriter{}, 100)),
		),
	)
	for _, version := range changelog.Versions() { // ascending
		raw, err := changelog.Get(version)
		if err != nil {
			return nil, err
		}
		release, err := render(md, version, raw)
		if err != nil {
			return nil, fmt.Errorf("changelog %s: %w", version, err)
		}
		s.byNum[release.Version] = release
		s.Releases = append(s.Releases, release)
	}
	if len(s.Releases) == 0 {
		return nil, fmt.Errorf("no release notes found")
	}
	for i, j := 0, len(s.Releases)-1; i < j; i, j = i+1, j-1 { // newest first
		s.Releases[i], s.Releases[j] = s.Releases[j], s.Releases[i]
	}
	return s, nil
}

// Release returns the release for version, which may be given with or
// without the leading "v", for example "v271" or "271".
func (s *Site) Release(version string) (*Release, bool) {
	n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(version), "v"))
	if err != nil {
		return nil, false
	}
	r, ok := s.byNum["v"+strconv.Itoa(n)]
	return r, ok
}

// Latest is the newest release.
func (s *Site) Latest() *Release { return s.Releases[0] }

var dateInParens = regexp.MustCompile(`\(([^)]+)\)$`)

func render(md goldmark.Markdown, version, raw string) (*Release, error) {
	body := []byte(raw)
	pc := parser.NewContext()
	doc := md.Parser().Parse(text.NewReader(body), parser.WithContext(pc))

	release := &Release{Version: version, Source: "changelog/" + version + ".md"}

	// The template shows the title, so drop the note's own first heading.
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if h, ok := n.(*ast.Heading); ok {
			release.Title = nodeText(h, body)
			doc.RemoveChild(doc, h)
			break
		}
	}
	if release.Title == "" {
		release.Title = version
	}
	if m := dateInParens.FindStringSubmatch(release.Title); m != nil {
		release.Date = strings.TrimSpace(m[1])
	}

	release.Summary = firstParagraphText(doc, body)

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if h, ok := n.(*ast.Heading); ok && entering && (h.Level == 2 || h.Level == 3) {
			if hid, ok := h.AttributeString("id"); ok {
				release.TOC = append(release.TOC, docsite.Heading{Level: h.Level, ID: string(hid.([]byte)), Text: nodeText(h, body)})
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, body, doc); err != nil {
		return nil, err
	}
	// goldmark escapes raw HTML in the Markdown, so its output is safe to
	// put in the page as is.
	release.HTML = template.HTML(buf.String())
	return release, nil
}

// firstParagraphText is the first paragraph's text, truncated for listings.
func firstParagraphText(doc ast.Node, src []byte) string {
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if _, ok := n.(*ast.Paragraph); ok {
			return truncate(strings.TrimSpace(nodeText(n, src)), 160)
		}
	}
	return ""
}

// truncate shortens s to at most max bytes, at a word boundary.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	if i := strings.LastIndex(cut, " "); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:") + "…"
}

// nodeText joins the text under n.
func nodeText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(src))
		case *ast.String:
			b.Write(t.Value)
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

// linkRewriter opens links that leave the site in a new tab.
type linkRewriter struct{}

func (linkRewriter) Transform(doc *ast.Document, _ text.Reader, pc parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Link:
			if u, err := url.Parse(string(node.Destination)); err == nil &&
				(u.Scheme == "http" || u.Scheme == "https") {
				node.SetAttributeString("target", []byte("_blank"))
				node.SetAttributeString("rel", []byte("noopener"))
			}
		case *ast.AutoLink:
			// Bare URLs are always external.
			node.SetAttributeString("target", []byte("_blank"))
			node.SetAttributeString("rel", []byte("noopener"))
		}
		return ast.WalkContinue, nil
	})
}
