package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestServeDirectory_IncludesHiddenFiles verifies that the HTTP directory
// listing handler includes dot-prefixed entries in its response.
func TestServeDirectory_IncludesHiddenFiles(t *testing.T) {
	root := makeTempDir(t)

	// Temporarily set ShowAllFiles so non-.md files aren't filtered either.
	origShowAll := ShowAllFiles
	ShowAllFiles = true
	t.Cleanup(func() { ShowAllFiles = origShowAll })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	serveDirectory(rec, req, root, "/", root)

	body := rec.Body.String()
	for _, want := range []string{".hidden.md", ".hiddendir"} {
		if !strings.Contains(body, want) {
			t.Errorf("serveDirectory response missing %q\nbody:\n%s", want, body)
		}
	}
}
