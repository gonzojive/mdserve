package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"mdserve/gitignore"
)

// GitDirName is the folder name used by git.
const GitDirName = ".git"

// GitIgnoreInstance holds the loaded gitignore patterns for the root directory.
var GitIgnoreInstance *gitignore.GitIgnore

// InitGitIgnore resolves and loads a .gitignore file if it exists in the rootDir.
func InitGitIgnore(rootDir string) {
	gi, err := gitignore.Load(filepath.Join(rootDir, ".gitignore"))
	if err != nil {
		log.Printf("Error loading .gitignore: %v", err)
	}
	GitIgnoreInstance = gi
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
	isDir := false
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		isDir = true
	}
	if GitIgnoreInstance != nil && GitIgnoreInstance.Match(path, isDir) {
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
	if GitIgnoreInstance != nil && GitIgnoreInstance.Match(relPath, true) {
		return false
	}
	return true
}
