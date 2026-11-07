package docsite

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mostlygeek/llama-swap/docs"
)

func TestDocsite_RewriteLink(t *testing.T) {
	const src = "kb/guides/model-runtime/writing-cmd.md"
	cases := []struct {
		dest     string
		want     string
		external bool
	}{
		{"../routing/groups-and-matrix.md", "/docs/guides/routing/groups-and-matrix", false},
		{"../routing/groups-and-matrix.md#matrix", "/docs/guides/routing/groups-and-matrix#matrix", false},
		{"../../../config.example.yaml", "/docs/reference/config", false},
		{"../../../README.md", "/docs/reference/readme", false},
		{"#heading", "#heading", false},
		{"https://example.com/x", "https://example.com/x", true},
		{"../../../../cmd/wol-proxy/README.md", "https://github.com/mostlygeek/llama-swap/blob/main/cmd/wol-proxy/README.md", true},
		{"/config-schema.json", "https://github.com/mostlygeek/llama-swap/blob/main/config-schema.json", true},
	}
	for _, c := range cases {
		got, external := rewriteLink(src, c.dest)
		if got != c.want || external != c.external {
			t.Errorf("rewriteLink(%q) = %q, %v; want %q, %v", c.dest, got, external, c.want, c.external)
		}
	}
}

func TestDocsite_RendersArticle(t *testing.T) {
	fsys := fstest.MapFS{
		"kb/guides/routing/a.md": {Data: []byte("---\ntitle: Article A\nsummary: About A.\ncategory: guides\n---\n\n# Article A\n\nSee [B](../tools/b.md) and <script>alert(1)</script>.\n\n## First part\n\n```yaml\nmodels: {}\n```\n\n### Detail\n\nText.\n")},
		"kb/guides/tools/b.md":   {Data: []byte("---\ntitle: B\n---\n\n# B\n\nBody.\n")},
		"config.example.yaml":    {Data: []byte("# header\n\n# models: the models\n# - required\nmodels:\n  a:\n    cmd: x\n\n# peers: other servers\npeers: {}\n")},
	}
	s, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	a, ok := s.Page("guides/routing/a")
	if !ok {
		t.Fatal("page missing")
	}
	html := string(a.HTML)
	if a.Title != "Article A" || a.Group != "Routing" || a.Summary != "About A." {
		t.Errorf("page = %+v", a)
	}
	if strings.Contains(html, "<h1") {
		t.Error("the article's own h1 should be dropped; the template shows the title")
	}
	if !strings.Contains(html, `href="/docs/guides/tools/b"`) {
		t.Errorf("relative link not rewritten:\n%s", html)
	}
	if strings.Contains(html, "<script>") {
		t.Error("raw HTML in Markdown must be escaped")
	}
	if !strings.Contains(html, `class="chroma"`) {
		t.Error("code block not highlighted")
	}
	if len(a.TOC) != 2 || a.TOC[0].Text != "First part" || a.TOC[0].ID != "first-part" || a.TOC[1].Level != 3 {
		t.Errorf("TOC = %+v", a.TOC)
	}

	if len(s.Config) != 2 || s.Config[0].Key != "models" || s.Config[0].Summary != "the models" {
		t.Errorf("config sections = %+v", s.Config)
	}
	if got := s.Links()["reference/config/peers"]; got != "/docs/reference/config#peers" {
		t.Errorf("config link = %q", got)
	}
}

// The real llama-swap docs must all render, with a title and a group.
func TestDocsite_LlamaSwapDocs(t *testing.T) {
	s, err := Load(docs.Files)
	if err != nil {
		t.Fatal(err)
	}
	listed := 0
	for _, g := range s.Nav {
		for _, p := range g.Pages {
			listed++
			// The config reference renders its sections from s.Config.
			if p.Title == "" || p.Title == p.ID || (p.HTML == "" && p.ID != ConfigID) {
				t.Errorf("%s: title %q, %d bytes of HTML", p.ID, p.Title, len(p.HTML))
			}
		}
	}
	if listed < 30 {
		t.Errorf("only %d pages listed", listed)
	}
	if _, ok := s.Page("guides/model-runtime/ttl-and-unloading"); !ok {
		t.Error("ttl-and-unloading missing")
	}
	if len(s.Config) < 10 {
		t.Errorf("only %d config sections", len(s.Config))
	}
	if !strings.Contains(s.HighlightCSS, "prefers-color-scheme: dark") {
		t.Error("highlight CSS has no dark theme")
	}
}
