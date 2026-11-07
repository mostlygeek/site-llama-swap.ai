// Package docsite renders llama-swap's documentation as HTML pages.
//
// The Markdown comes from llama-swap's docs package (the knowledge base, the
// README and config.example.yaml), at the llama-swap version the site is
// built with. Everything is rendered once, when the site starts: the docs
// only change when the binary does.
//
// Pages use the same ids as the Help agent's docs__* tools, such as
// guides/model-runtime/ttl-and-unloading, so the agent can link to them.
package docsite

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"gopkg.in/yaml.v3"
)

// Paths inside llama-swap's docs package.
const (
	kbDir         = "kb"
	configExample = "config.example.yaml"
	readme        = "README.md"
)

// ConfigID is the page id of the config.example.yaml reference.
const ConfigID = "reference/config"

// readmeID matches the id llama-swap's docs tools give the README.
const readmeID = "reference/readme"

// repoBlob is where links to files outside the docs point.
const repoBlob = "https://github.com/mostlygeek/llama-swap/blob/main/"

// Page is one rendered document.
type Page struct {
	ID       string
	Title    string
	Summary  string
	Category string
	// Group is the navigation heading the page is listed under.
	Group string
	HTML  template.HTML
	// TOC lists the page's second and third level headings.
	TOC []Heading
	// Source is the file's path in the llama-swap repository.
	Source string
	// Hidden pages are served but not listed in the navigation.
	Hidden bool
}

// URL is the page's path on the site.
func (p *Page) URL() string { return "/docs/" + p.ID }

// SourceURL links to the page's Markdown on GitHub.
func (p *Page) SourceURL() string { return repoBlob + p.Source }

// Heading is one entry of a page's table of contents.
type Heading struct {
	Level int
	ID    string
	Text  string
}

// NavGroup is one heading of the navigation and its pages.
type NavGroup struct {
	Name  string
	Pages []*Page
}

// ConfigSection is one top-level key of config.example.yaml.
type ConfigSection struct {
	Key     string
	Summary string
	HTML    template.HTML
}

// Site holds every rendered page.
type Site struct {
	pages map[string]*Page
	// Nav lists the pages by group, in reading order.
	Nav []NavGroup
	// ConfigIntro and Config are the config.example.yaml reference.
	ConfigIntro template.HTML
	Config      []ConfigSection
	// HighlightCSS colours the code blocks in light and dark themes.
	HighlightCSS string

	md goldmark.Markdown
}

// frontmatter is the YAML block at the top of a knowledge base article.
type frontmatter struct {
	Title    string `yaml:"title"`
	Summary  string `yaml:"summary"`
	Category string `yaml:"category"`
}

// Load reads and renders the documentation in fsys, laid out like
// llama-swap's docs directory.
func Load(fsys fs.FS) (*Site, error) {
	s := &Site{pages: map[string]*Page{}}
	s.md = goldmark.New(
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

	err := fs.WalkDir(fsys, kbDir, func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(name, ".md") || strings.EqualFold(d.Name(), "README.md") {
			return err
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, kbDir+"/"), ".md")
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		page, err := s.renderArticle(id, name, raw)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		s.pages[id] = page
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(s.pages) == 0 {
		return nil, fmt.Errorf("no documents found under %s", kbDir)
	}

	if raw, err := fs.ReadFile(fsys, readme); err == nil {
		page, err := s.renderArticle(readmeID, readme, raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", readme, err)
		}
		// The README is a work in progress about running the docs agent
		// locally; serve it for the agent's links but do not list it.
		page.Hidden = true
		page.Group = "Reference"
		s.pages[readmeID] = page
	}

	raw, err := fs.ReadFile(fsys, configExample)
	if err != nil {
		return nil, err
	}
	if err := s.renderConfig(string(raw)); err != nil {
		return nil, fmt.Errorf("%s: %w", configExample, err)
	}
	s.pages[ConfigID] = &Page{
		ID:       ConfigID,
		Title:    "Configuration reference",
		Summary:  "Every configuration option, from the annotated config.example.yaml.",
		Category: "reference",
		Group:    "Reference",
		Source:   "docs/" + configExample,
	}

	s.buildNav()
	if s.HighlightCSS, err = highlightCSS(); err != nil {
		return nil, err
	}
	return s, nil
}

// Page returns the page with the given id.
func (s *Site) Page(id string) (*Page, bool) {
	p, ok := s.pages[id]
	return p, ok
}

// Links maps every id the Help agent can name to its URL on the site,
// including one per config.example.yaml section (reference/config/models).
func (s *Site) Links() map[string]string {
	links := map[string]string{}
	for id, p := range s.pages {
		links[id] = p.URL()
	}
	for _, c := range s.Config {
		links[ConfigID+"/"+c.Key] = URL(ConfigID + "/" + c.Key)
	}
	return links
}

// URL turns a docs tool id into a site path. Config section ids point at an
// anchor on the reference page.
func URL(id string) string {
	if key, ok := strings.CutPrefix(id, ConfigID+"/"); ok {
		if key == "" || strings.HasPrefix(key, "_") {
			return "/docs/" + ConfigID
		}
		return "/docs/" + ConfigID + "#" + key
	}
	return "/docs/" + id
}

func (s *Site) renderArticle(id, source string, raw []byte) (*Page, error) {
	var fm frontmatter
	body := raw
	if rest, ok := bytes.CutPrefix(raw, []byte("---\n")); ok {
		if block, after, ok := bytes.Cut(rest, []byte("\n---\n")); ok {
			if err := yaml.Unmarshal(block, &fm); err != nil {
				return nil, fmt.Errorf("front matter: %w", err)
			}
			body = after
		}
	}

	page := &Page{
		ID:       id,
		Title:    fm.Title,
		Summary:  fm.Summary,
		Category: fm.Category,
		Group:    groupName(id),
		Source:   "docs/" + source,
	}

	pc := parser.NewContext()
	pc.Set(sourceKey, source)
	doc := s.md.Parser().Parse(text.NewReader(body), parser.WithContext(pc))

	// The template shows the title, so drop the article's own first heading.
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if h, ok := n.(*ast.Heading); ok && h.Level == 1 {
			if page.Title == "" {
				page.Title = nodeText(h, body)
			}
			doc.RemoveChild(doc, h)
			break
		}
	}
	if page.Title == "" {
		page.Title = id
	}

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if h, ok := n.(*ast.Heading); ok && entering && (h.Level == 2 || h.Level == 3) {
			if hid, ok := h.AttributeString("id"); ok {
				page.TOC = append(page.TOC, Heading{Level: h.Level, ID: string(hid.([]byte)), Text: nodeText(h, body)})
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})

	var buf bytes.Buffer
	if err := s.md.Renderer().Render(&buf, body, doc); err != nil {
		return nil, err
	}
	// goldmark escapes raw HTML in the Markdown, so its output is safe to
	// put in the page as is.
	page.HTML = template.HTML(buf.String())
	return page, nil
}

var topLevelKey = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*):`)

// renderConfig splits config.example.yaml at its top-level keys, each with
// the comment block above it, the same way llama-swap's docs tools do.
func (s *Site) renderConfig(content string) error {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	type bounds struct {
		key, summary string
		start, end   int
	}
	var found []bounds
	for i, line := range lines {
		m := topLevelKey.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		start := i
		for start > 0 && strings.HasPrefix(lines[start-1], "#") {
			start--
		}
		found = append(found, bounds{key: m[1], summary: sectionSummary(lines[start:i], m[1]), start: start})
	}
	if len(found) == 0 {
		return fmt.Errorf("no top-level keys found")
	}
	for i := range found {
		if i+1 < len(found) {
			found[i].end = found[i+1].start
		} else {
			found[i].end = len(lines)
		}
	}

	yamlBlock := func(lines []string) (template.HTML, error) {
		var buf bytes.Buffer
		src := "```yaml\n" + strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n```\n"
		if err := s.md.Convert([]byte(src), &buf); err != nil {
			return "", err
		}
		return template.HTML(buf.String()), nil
	}

	var err error
	if s.ConfigIntro, err = yamlBlock(lines[:found[0].start]); err != nil {
		return err
	}
	for _, b := range found {
		html, err := yamlBlock(lines[b.start:b.end])
		if err != nil {
			return err
		}
		s.Config = append(s.Config, ConfigSection{Key: b.key, Summary: b.summary, HTML: html})
	}
	return nil
}

// sectionSummary is the first comment line describing a key, without the
// "key:" prefix.
func sectionSummary(comments []string, key string) string {
	for _, line := range comments {
		t := strings.TrimSpace(strings.TrimPrefix(line, "#"))
		t = strings.TrimSpace(strings.TrimPrefix(t, key+":"))
		if t != "" && t != "-" {
			return t
		}
	}
	return ""
}

// groupOrder lists the navigation groups in reading order. Groups not named
// here follow, alphabetically.
var groupOrder = []string{
	"Configuration", "Model runtime", "Routing", "API integration", "Connectivity",
	"Operations", "Upstreams", "Tools", "Examples", "Reference",
}

// groupName names the navigation group for an id: the folder under guides/,
// or the top-level folder otherwise.
func groupName(id string) string {
	parts := strings.Split(id, "/")
	name := parts[0]
	if parts[0] == "guides" && len(parts) > 2 {
		name = parts[1]
	}
	words := strings.Split(name, "-")
	for i, w := range words {
		if w == "api" {
			words[i] = "API"
		}
	}
	name = strings.Join(words, " ")
	return strings.ToUpper(name[:1]) + name[1:]
}

func (s *Site) buildNav() {
	groups := map[string][]*Page{}
	for _, p := range s.pages {
		if !p.Hidden {
			groups[p.Group] = append(groups[p.Group], p)
		}
	}
	rank := func(name string) int {
		for i, g := range groupOrder {
			if g == name {
				return i
			}
		}
		return len(groupOrder)
	}
	var names []string
	for name := range groups {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		ri, rj := rank(names[i]), rank(names[j])
		if ri != rj {
			return ri < rj
		}
		return names[i] < names[j]
	})
	for _, name := range names {
		pages := groups[name]
		sort.Slice(pages, func(i, j int) bool { return pages[i].Title < pages[j].Title })
		s.Nav = append(s.Nav, NavGroup{Name: name, Pages: pages})
	}
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

// highlightCSS returns the code colours for light pages, and for dark pages
// under prefers-color-scheme.
func highlightCSS() (string, error) {
	formatter := chromahtml.New(chromahtml.WithClasses(true))
	var light, dark bytes.Buffer
	if err := formatter.WriteCSS(&light, styles.Get("github")); err != nil {
		return "", err
	}
	if err := formatter.WriteCSS(&dark, styles.Get("github-dark")); err != nil {
		return "", err
	}
	// The site's stylesheet sets the code block background.
	return light.String() + "\n@media (prefers-color-scheme: dark) {\n" + dark.String() + "}\n" +
		".chroma { background-color: transparent !important; }\n", nil
}
