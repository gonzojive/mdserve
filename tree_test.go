package main

import (
	"os"
	"path/filepath"
	"testing"
)

// makeTempDir creates a temporary directory tree for testing:
//
//	root/
//	  visible.md
//	  .hidden.md
//	  .hiddendir/
//	    inside.md
//	  .git/
//	    config
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
	must(os.Mkdir(filepath.Join(root, ".git"), 0o755))
	must(os.WriteFile(filepath.Join(root, ".git", "config"), []byte("[core]"), 0o644))
	must(os.WriteFile(filepath.Join(root, ".gitignore"), []byte("/bazel-*\n*.swp\n"), 0o644))
	must(os.Mkdir(filepath.Join(root, "bazel-out"), 0o755))
	must(os.WriteFile(filepath.Join(root, "bazel-out", "ignored.md"), []byte("# ignored"), 0o644))
	must(os.WriteFile(filepath.Join(root, "temp.swp"), []byte("temp"), 0o644))

	InitGitIgnore(root)
	return root
}

// TestBuildFileTree_IncludesHiddenFiles verifies that buildFileTree returns
// dot-prefixed files and directories but excludes .git.
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
	if names[".git"] {
		t.Error("expected .git directory to be excluded from file tree")
	}
	if names["config"] {
		t.Error("expected config file inside .git to be excluded from file tree")
	}
	if names[".gitignore"] {
		t.Error("expected .gitignore file to be excluded from file tree")
	}
	if names["bazel-out"] || names["ignored.md"] {
		t.Error("expected bazel-out and ignored.md to be excluded from file tree")
	}
	if names["temp.swp"] {
		t.Error("expected temp.swp to be excluded from file tree")
	}
}

// TestMakeBreadcrumbs verifies the split and cumulative path generation logic.
func TestMakeBreadcrumbs(t *testing.T) {
	breadcrumbs := makeBreadcrumbs("/a/b/c")
	if len(breadcrumbs) != 4 {
		t.Fatalf("Expected 4 breadcrumbs, got %d", len(breadcrumbs))
	}

	if breadcrumbs[0].Name != "Root" || breadcrumbs[0].Path != "/" {
		t.Errorf("First breadcrumb mismatch: %+v", breadcrumbs[0])
	}
	if breadcrumbs[1].Name != "a" || breadcrumbs[1].Path != "/a" {
		t.Errorf("Second breadcrumb mismatch: %+v", breadcrumbs[1])
	}
	if breadcrumbs[2].Name != "b" || breadcrumbs[2].Path != "/a/b" {
		t.Errorf("Third breadcrumb mismatch: %+v", breadcrumbs[2])
	}
	if breadcrumbs[3].Name != "c" || breadcrumbs[3].Path != "/a/b/c" {
		t.Errorf("Fourth breadcrumb mismatch: %+v", breadcrumbs[3])
	}
}

// TestBuildFileTree_IncludesGitWithShowAll verifies that buildFileTree returns
// .git when showAll is true.
func TestBuildFileTree_IncludesGitWithShowAll(t *testing.T) {
	root := makeTempDir(t)

	tree, err := buildFileTree(root, true)
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

	if !names[".git"] {
		t.Error("expected .git directory to be included in file tree when showAll is true")
	}
	if !names["config"] {
		t.Error("expected config file inside .git to be included in file tree when showAll is true")
	}
}
