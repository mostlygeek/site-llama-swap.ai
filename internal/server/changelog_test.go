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
		{"/releases", 200, `href="/releases/v271"`},
		{"/releases/", 200, `href="/releases/v200"`},
		{"/releases/v271", 200, "Nov 8, 2026"},
		{"/releases/v271", 200, `href="https://github.com/mostlygeek/llama-swap/blob/main/changelog/v271.md"`},
		{"/releases/v200", 200, "PR #630"},
		{"/releases/v99999", 404, "Page not found"},
	}
	for _, c := range cases {
		w := get(t, s, c.path)
		if w.Code != c.status || !strings.Contains(w.Body.String(), c.want) {
			t.Errorf("GET %s = %d, want %d containing %q", c.path, w.Code, c.status, c.want)
		}
	}

	// The index lists the releases newest first.
	body := get(t, s, "/releases").Body.String()
	if newest, oldest := strings.Index(body, `href="/releases/v271"`), strings.Index(body, `href="/releases/v200"`); newest == -1 || oldest == -1 || newest > oldest {
		t.Errorf("releases not listed newest first (v271 at %d, v200 at %d)", newest, oldest)
	}

	// The old /changelog paths redirect to /releases.
	for path, want := range map[string]string{"/changelog": "/releases", "/changelog/": "/releases/", "/changelog/v271": "/releases/v271"} {
		w := get(t, s, path)
		if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != want {
			t.Errorf("%s = %d %s, want a redirect to %s", path, w.Code, w.Header().Get("Location"), want)
		}
	}

	// The version may be given without the leading "v".
	w := get(t, s, "/releases/271")
	if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != "/releases/v271" {
		t.Errorf("/releases/271 = %d %s, want a redirect to /releases/v271", w.Code, w.Header().Get("Location"))
	}

	if !strings.Contains(get(t, s, "/").Body.String(), `href="/releases"`) {
		t.Error("header does not link to /releases")
	}
}
