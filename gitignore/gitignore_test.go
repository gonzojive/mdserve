package gitignore

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGitIgnoreMatch verifies loading patterns and matching paths.
func TestGitIgnoreMatch(t *testing.T) {
	tempDir := t.TempDir()
	gitignorePath := filepath.Join(tempDir, ".gitignore")
	content := `# comment
/mdserve
*.swp
/bazel-*
!/bazel-keep
`
	if err := os.WriteFile(gitignorePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp .gitignore: %v", err)
	}

	gi, err := Load(gitignorePath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	tests := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"mdserve", false, true},
		{"sub/mdserve", false, false}, // root-only pattern
		{"file.swp", false, true},
		{"sub/file.swp", false, true}, // glob pattern anywhere
		{"bazel-bin", true, true},
		{"bazel-out/main", false, true}, // root-only glob prefix match
		{"bazel-keep", false, false},    // negated pattern
		{"normal.md", false, false},
	}
	for _, tt := range tests {
		if got := gi.Match(tt.path, tt.isDir); got != tt.want {
			t.Errorf("gi.Match(%q, %v) = %v; want %v", tt.path, tt.isDir, got, tt.want)
		}
	}
}
