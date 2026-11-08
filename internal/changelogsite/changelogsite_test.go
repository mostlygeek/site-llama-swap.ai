package changelogsite

import (
	"strings"
	"testing"

	"github.com/mostlygeek/llama-swap/changelog"
)

func TestChangelogsite_Load(t *testing.T) {
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s.Releases) != len(changelog.Versions()) {
		t.Fatalf("got %d releases, want %d", len(s.Releases), len(changelog.Versions()))
	}

	// newest first
	for i := 1; i < len(s.Releases); i++ {
		if s.Releases[i-1].Version < s.Releases[i].Version {
			t.Fatalf("releases not newest first at %d: %s then %s", i, s.Releases[i-1].Version, s.Releases[i].Version)
		}
	}

	latest, err := changelog.Latest()
	if err != nil {
		t.Fatalf("changelog.Latest: %v", err)
	}
	if got := s.Latest().Version; got != latest {
		t.Errorf("Latest = %s, want %s", got, latest)
	}

	// every release has a title, a summary and rendered HTML
	for _, r := range s.Releases {
		if r.Title == "" || r.Summary == "" || r.HTML == "" {
			t.Errorf("%s: missing title/summary/HTML: %q / %q / %d bytes", r.Version, r.Title, r.Summary, len(r.HTML))
		}
		if r.URL() != "/changelog/"+r.Version {
			t.Errorf("%s: URL = %s", r.Version, r.URL())
		}
	}
}

func TestChangelogsite_Lookup(t *testing.T) {
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, version := range []string{"v271", "271", " v271 "} {
		r, ok := s.Release(version)
		if !ok || r.Version != "v271" {
			t.Errorf("Release(%q) = %v, %v; want v271", version, r, ok)
		}
	}
	for _, version := range []string{"v99999", "abc", ""} {
		if _, ok := s.Release(version); ok {
			t.Errorf("Release(%q) found, want not found", version)
		}
	}
}

func TestChangelogsite_Render(t *testing.T) {
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r, _ := s.Release("v200")

	// The title heading is dropped from the HTML and shows in Title.
	if strings.Contains(string(r.HTML), "<h2>v200") {
		t.Error("HTML still contains the title heading")
	}
	if !strings.HasPrefix(r.Title, "v200") {
		t.Errorf("Title = %q, want it to start with v200", r.Title)
	}

	// GitHub links open in a new tab.
	if !strings.Contains(string(r.HTML), `href="https://github.com/mostlygeek/llama-swap/pull/630" target="_blank"`) {
		t.Errorf("HTML has no new-tab PR link:\n%s", string(r.HTML)[:min(600, len(r.HTML))])
	}

	// The source links to the note on GitHub.
	if r.SourceURL() != "https://github.com/mostlygeek/llama-swap/blob/main/changelog/v200.md" {
		t.Errorf("SourceURL = %s", r.SourceURL())
	}
}
