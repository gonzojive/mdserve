package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestStripComments verifies that stripComments successfully removes both
// single-line and multi-line comments from JSONC bytes.
func TestStripComments(t *testing.T) {
	input := []byte(`{
		// This is a single-line comment
		"install_dir": "/path/to/install", /* This is a
multi-line comment */
		"other": "value"
	}`)

	expected := `{
		
		"install_dir": "/path/to/install", 
		"other": "value"
	}`

	output := stripComments(input)
	if string(output) != expected {
		t.Errorf("Expected cleaned config:\n%q\nGot:\n%q", expected, string(output))
	}
}

// TestDetermineInstallDir verifies the fallback logic of determineInstallDir.
func TestDetermineInstallDir(t *testing.T) {
	// Case 1: Configured path (with tilde expansion)
	cfg := &Config{
		InstallDir: "~/custom/path",
	}

	dir, err := determineInstallDir(cfg)
	if err != nil {
		t.Fatalf("Failed to determine install dir: %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, "custom/path")
	if dir != expected {
		t.Errorf("Expected path %q, got %q", expected, dir)
	}

	// Case 2: Empty config path defaults to ~/.local/bin or ~/bin depending on existence.
	// Since we cannot guarantee local existence of those directories during unit test execution,
	// we just verify that it returns one of the two paths without returning an error.
	cfgEmpty := &Config{}
	defaultDir, err := determineInstallDir(cfgEmpty)
	if err != nil {
		t.Fatalf("Failed on empty config: %v", err)
	}

	expectedLocalBin := filepath.Join(home, ".local", "bin")
	expectedUserBin := filepath.Join(home, "bin")
	if defaultDir != expectedLocalBin && defaultDir != expectedUserBin {
		t.Errorf("Expected default dir to be %q or %q, but got %q", expectedLocalBin, expectedUserBin, defaultDir)
	}
}
