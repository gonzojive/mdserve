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

// TestServeDirectory_ExcludeGit verifies that .git is excluded or included
// in directory listings depending on ShowAllFiles.
func TestServeDirectory_ExcludeGit(t *testing.T) {
	root := makeTempDir(t)

	// Case 1: ShowAllFiles is false (default) -> .git is excluded
	{
		origShowAll := ShowAllFiles
		ShowAllFiles = false
		defer func() { ShowAllFiles = origShowAll }()

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		serveDirectory(rec, req, root, "/", root)

		body := rec.Body.String()
		if strings.Contains(body, ".git") {
			t.Error("expected .git directory to be excluded when ShowAllFiles is false")
		}
	}

	// Case 2: ShowAllFiles is true -> .git is included
	{
		origShowAll := ShowAllFiles
		ShowAllFiles = true
		defer func() { ShowAllFiles = origShowAll }()

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		serveDirectory(rec, req, root, "/", root)

		body := rec.Body.String()
		if !strings.Contains(body, ".git") {
			t.Error("expected .git directory to be included when ShowAllFiles is true")
		}
	}
}
