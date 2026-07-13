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
	// Set up temporary gitignore patterns
	GitIgnoreInstance = &GitIgnore{
		patterns: []string{
			"/bazel-*",
			"*.swp",
		},
	}
	defer func() { GitIgnoreInstance = nil }()

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
		{"foo/bar/git", false, false},
		{"foo/.git-info", false, false},
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
	GitIgnoreInstance = &GitIgnore{
		patterns: []string{
			"/bazel-*",
		},
	}
	defer func() { GitIgnoreInstance = nil }()

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

// TestGitIgnoreMatch verifies loading patterns and matching paths.
func TestGitIgnoreMatch(t *testing.T) {
	tempDir := t.TempDir()
	gitignorePath := filepath.Join(tempDir, ".gitignore")
	content := `# comment
/mdserve
*.swp
/bazel-*
`
	if err := os.WriteFile(gitignorePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp .gitignore: %v", err)
	}

	gi, err := LoadGitIgnore(gitignorePath)
	if err != nil {
		t.Fatalf("LoadGitIgnore failed: %v", err)
	}

	tests := []struct {
		path string
		want bool
	}{
		{"mdserve", true},
		{"sub/mdserve", false}, // root-only pattern
		{"file.swp", true},
		{"sub/file.swp", true}, // glob pattern anywhere
		{"bazel-bin", true},
		{"bazel-out/main", true}, // root-only glob prefix match
		{"normal.md", false},
	}
	for _, tt := range tests {
		if got := gi.Match(tt.path); got != tt.want {
			t.Errorf("gi.Match(%q) = %v; want %v", tt.path, got, tt.want)
		}
	}
}
