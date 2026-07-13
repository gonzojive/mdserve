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

// RepoRootPath is the resolved repository root (or rootDir if not in a repository).
var RepoRootPath string

// GitIgnoreInstance holds the loaded gitignore patterns for the root directory.
var GitIgnoreInstance *gitignore.GitIgnore

// FindRepoRoot walks up from startDir to find the repository root directory (containing .git or .gitignore).
func FindRepoRoot(startDir string) string {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return startDir
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return startDir
}

// InitGitIgnore resolves and loads a .gitignore file by searching upwards from rootDir.
func InitGitIgnore(rootDir string) {
	RepoRootPath = FindRepoRoot(rootDir)
	gi, err := gitignore.Load(filepath.Join(RepoRootPath, ".gitignore"))
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
	// Compute relative path from RepoRootPath
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	relPath, err := filepath.Rel(RepoRootPath, absPath)
	if err != nil {
		relPath = path
	}

	isDir := false
	if info, err := os.Stat(absPath); err == nil && info.IsDir() {
		isDir = true
	}
	if GitIgnoreInstance != nil && GitIgnoreInstance.Match(relPath, isDir) {
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
