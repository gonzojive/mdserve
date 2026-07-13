package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIsGitDir verifies that IsGitDir identifies .git correctly.
func TestIsGitDir(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{".git", true},
		{".git/", false},
		{"git", false},
		{".gitignore", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsGitDir(tt.name); got != tt.want {
			t.Errorf("IsGitDir(%q) = %v; want %v", tt.name, got, tt.want)
		}
	}
}

// TestShouldExcludeName verifies ShouldExcludeName filters .git and .gitignore when not showing all files.
func TestShouldExcludeName(t *testing.T) {
	tests := []struct {
		name    string
		showAll bool
		want    bool
	}{
		{".git", false, true},
		{".git", true, false},
		{".gitignore", false, true},
		{".gitignore", true, false},
		{"visible.md", false, false},
	}
	for _, tt := range tests {
		if got := ShouldExcludeName(tt.name, tt.showAll); got != tt.want {
			t.Errorf("ShouldExcludeName(%q, %v) = %v; want %v", tt.name, tt.showAll, got, tt.want)
		}
	}
}

// TestShouldExcludePath verifies ShouldExcludePath detects .git, .gitignore, and ignored files.
func TestShouldExcludePath(t *testing.T) {
	tempRoot := t.TempDir()
	origWd, _ := os.Getwd()
	_ = os.Chdir(tempRoot)
	defer func() {
		_ = os.Chdir(origWd)
		GitIgnoreInstance = nil
	}()

	// Write mock gitignore
	_ = os.WriteFile(".gitignore", []byte("/bazel-*\n*.swp\n"), 0644)
	InitGitIgnore(tempRoot)

	_ = os.Mkdir("bazel-out", 0755)
	_ = os.WriteFile("bazel-out/file.md", []byte("bazel"), 0644)
	_ = os.WriteFile("file.swp", []byte("swp"), 0644)
	_ = os.Mkdir("foo", 0755)
	_ = os.WriteFile("foo/file.swp", []byte("swp"), 0644)
	_ = os.WriteFile("foo/bar", []byte("bar"), 0644)

	tests := []struct {
		path    string
		showAll bool
		want    bool
	}{
		{".git", false, true},
		{".git/config", false, true},
		{"foo/.git/config", false, true},
		{".gitignore", false, true},
		{"foo/.gitignore", false, true},
		{"bazel-out/file.md", false, true},
		{"file.swp", false, true},
		{"foo/file.swp", false, true},
		{"foo/bar", false, false},
		{".git", true, false},
		{".git/config", true, false},
		{"bazel-out/file.md", true, false},
	}
	for _, tt := range tests {
		if got := ShouldExcludePath(tt.path, tt.showAll); got != tt.want {
			t.Errorf("ShouldExcludePath(%q, %v) = %v; want %v", tt.path, tt.showAll, got, tt.want)
		}
	}
}

// TestShouldWatchPath verifies ShouldWatchPath always filters .git and ignored files.
func TestShouldWatchPath(t *testing.T) {
	tempRoot := t.TempDir()
	defer func() { GitIgnoreInstance = nil }()

	_ = os.WriteFile(filepath.Join(tempRoot, ".gitignore"), []byte("/bazel-*\n"), 0644)
	InitGitIgnore(tempRoot)

	tests := []struct {
		path    string
		relPath string
		want    bool
	}{
		{".git", ".git", false},
		{".git/config", ".git/config", false},
		{"foo/.git", "foo/.git", false},
		{"bazel-out", "bazel-out", false},
		{"foo/bar", "foo/bar", true},
		{".github", ".github", true},
	}
	for _, tt := range tests {
		if got := ShouldWatchPath(tt.path, tt.relPath); got != tt.want {
			t.Errorf("ShouldWatchPath(%q, %q) = %v; want %v", tt.path, tt.relPath, got, tt.want)
		}
	}
}
