package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestChangelog_Pages(t *testing.T) {
	s := newTestServer(t)
	cases := []struct {
		path   string
		status int
		want   string
	}{
		{"/changelog", 200, `href="/changelog/v271"`},
		{"/changelog/", 200, `href="/changelog/v200"`},
		{"/changelog/v271", 200, "Nov 8, 2026"},
		{"/changelog/v271", 200, `href="https://github.com/mostlygeek/llama-swap/blob/main/changelog/v271.md"`},
		{"/changelog/v200", 200, "PR #630"},
		{"/changelog/v99999", 404, "Page not found"},
	}
	for _, c := range cases {
		w := get(t, s, c.path)
		if w.Code != c.status || !strings.Contains(w.Body.String(), c.want) {
			t.Errorf("GET %s = %d, want %d containing %q", c.path, w.Code, c.status, c.want)
		}
	}

	// The index lists the releases newest first.
	body := get(t, s, "/changelog").Body.String()
	if newest, oldest := strings.Index(body, `href="/changelog/v271"`), strings.Index(body, `href="/changelog/v200"`); newest == -1 || oldest == -1 || newest > oldest {
		t.Errorf("releases not listed newest first (v271 at %d, v200 at %d)", newest, oldest)
	}

	// The version may be given without the leading "v".
	w := get(t, s, "/changelog/271")
	if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != "/changelog/v271" {
		t.Errorf("/changelog/271 = %d %s, want a redirect to /changelog/v271", w.Code, w.Header().Get("Location"))
	}

	if !strings.Contains(get(t, s, "/").Body.String(), `href="/changelog"`) {
		t.Error("header does not link to /changelog")
	}
}
