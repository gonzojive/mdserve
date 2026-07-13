package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateMarkdownParser_Footnotes(t *testing.T) {
	parser := createMarkdownParser()
	input := []byte("Text with reference.[^1]\n\n[^1]: This is the footnote.")
	var buf bytes.Buffer
	if err := parser.Convert(input, &buf); err != nil {
		t.Fatalf("Failed to parse markdown: %v", err)
	}

	output := buf.String()

	// Check if in-text footnote reference was parsed
	if !strings.Contains(output, `class="footnote-ref"`) {
		t.Errorf("Expected output to contain 'class=\"footnote-ref\"', got:\n%s", output)
	}

	// Check if footnotes container was parsed
	if !strings.Contains(output, `class="footnotes"`) {
		t.Errorf("Expected output to contain 'class=\"footnotes\"', got:\n%s", output)
	}

	// Check if footnote content is present
	if !strings.Contains(output, "This is the footnote.") {
		t.Errorf("Expected output to contain footnote text, got:\n%s", output)
	}
}

// makeTempDir creates a temporary directory tree for testing:
//
//	root/
//	  visible.md
//	  .hidden.md
//	  .hiddendir/
//	    inside.md
func makeTempDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(root, "visible.md"), []byte("# visible"), 0o644))
	must(os.WriteFile(filepath.Join(root, ".hidden.md"), []byte("# hidden"), 0o644))
	must(os.Mkdir(filepath.Join(root, ".hiddendir"), 0o755))
	must(os.WriteFile(filepath.Join(root, ".hiddendir", "inside.md"), []byte("# inside"), 0o644))
	return root
}

// TestBuildFileTree_IncludesHiddenFiles verifies that buildFileTree returns
// dot-prefixed files and directories.
func TestBuildFileTree_IncludesHiddenFiles(t *testing.T) {
	root := makeTempDir(t)

	tree, err := buildFileTree(root, false)
	if err != nil {
		t.Fatalf("buildFileTree: %v", err)
	}

	names := map[string]bool{}
	var collect func(nodes []*FileNode)
	collect = func(nodes []*FileNode) {
		for _, n := range nodes {
			names[n.Name] = true
			collect(n.Children)
		}
	}
	collect(tree.Children)

	if !names[".hidden.md"] {
		t.Errorf("expected .hidden.md in file tree, got names: %v", names)
	}
	if !names[".hiddendir"] {
		t.Errorf("expected .hiddendir in file tree, got names: %v", names)
	}
	if !names["inside.md"] {
		t.Errorf("expected inside.md (inside .hiddendir) in file tree, got names: %v", names)
	}
}

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
