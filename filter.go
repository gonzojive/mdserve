package main

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// GitDirName is the folder name used by git.
const GitDirName = ".git"

// GitIgnoreInstance holds the loaded gitignore patterns for the root directory.
var GitIgnoreInstance *GitIgnore

// GitIgnore holds patterns parsed from a .gitignore file.
type GitIgnore struct {
	patterns []string
}

// InitGitIgnore resolves and loads a .gitignore file if it exists in the rootDir.
func InitGitIgnore(rootDir string) {
	gi, err := LoadGitIgnore(filepath.Join(rootDir, ".gitignore"))
	if err != nil {
		log.Printf("Error loading .gitignore: %v", err)
	}
	GitIgnoreInstance = gi
}

// LoadGitIgnore reads and parses a .gitignore file if it exists at the given path.
func LoadGitIgnore(path string) (*GitIgnore, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return &GitIgnore{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var patterns []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return &GitIgnore{patterns: patterns}, scanner.Err()
}

// Match checks if the given relative path matches any pattern in the .gitignore.
func (gi *GitIgnore) Match(relPath string) bool {
	// Standardize to forward slashes for matching
	relPath = filepath.ToSlash(filepath.Clean(relPath))
	if relPath == "." || relPath == "/" {
		return false
	}
	// Strip leading slash if any
	relPathTrimmed := strings.TrimPrefix(relPath, "/")

	for _, pattern := range gi.patterns {
		pat := filepath.ToSlash(pattern)
		// Handle root-only patterns starting with '/'
		isRootOnly := strings.HasPrefix(pat, "/")
		patTrimmed := strings.TrimPrefix(pat, "/")

		// If pattern ends with '/', it matches directory names.
		// For simplicity in matching, we can trim it.
		patTrimmed = strings.TrimSuffix(patTrimmed, "/")

		if isRootOnly {
			// Check if the whole relative path matches, or if its root segment matches
			matched, err := filepath.Match(patTrimmed, relPathTrimmed)
			if err == nil && matched {
				return true
			}
			parts := strings.Split(relPathTrimmed, "/")
			if len(parts) > 0 {
				matched, err := filepath.Match(patTrimmed, parts[0])
				if err == nil && matched {
					return true
				}
			}
		} else {
			// Matches anywhere in the path.
			segments := strings.Split(relPathTrimmed, "/")
			for _, segment := range segments {
				matched, err := filepath.Match(patTrimmed, segment)
				if err == nil && matched {
					return true
				}
			}
		}
	}
	return false
}

// IsGitDir checks if a directory or file name matches the git directory name.
func IsGitDir(name string) bool {
	return name == GitDirName
}

// ShouldExcludeName determines if a directory entry should be excluded from the sidebar tree or directory listing.
func ShouldExcludeName(name string, showAll bool) bool {
	if IsGitDir(name) || name == ".gitignore" {
		return !showAll
	}
	return false
}

// ShouldExcludePath determines if a file path contains a git directory segment or matches a gitignore pattern,
// and should be blocked from HTTP serving.
func ShouldExcludePath(path string, showAll bool) bool {
	if showAll {
		return false
	}
	segments := strings.Split(filepath.ToSlash(filepath.Clean(path)), "/")
	for _, segment := range segments {
		if IsGitDir(segment) || segment == ".gitignore" {
			return true
		}
	}
	if GitIgnoreInstance != nil && GitIgnoreInstance.Match(path) {
		return true
	}
	return false
}

// ShouldWatchPath determines if a directory path is safe for watching.
// Git metadata directories and gitignored paths are never watched to avoid reload loops during git/build operations.
func ShouldWatchPath(path string, relPath string) bool {
	segments := strings.Split(filepath.ToSlash(filepath.Clean(path)), "/")
	for _, segment := range segments {
		if IsGitDir(segment) {
			return false
		}
	}
	if GitIgnoreInstance != nil && GitIgnoreInstance.Match(relPath) {
		return false
	}
	return true
}
