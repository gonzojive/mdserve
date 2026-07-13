package main

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// GitDirName is the folder name used by git.
const GitDirName = ".git"

// GitIgnoreInstance holds the loaded gitignore patterns for the root directory.
var GitIgnoreInstance *GitIgnore

type gitIgnoreRule struct {
	regex  *regexp.Regexp
	negate bool
	isDir  bool
}

// GitIgnore holds patterns parsed and compiled from a .gitignore file.
type GitIgnore struct {
	rules []gitIgnoreRule
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

	var rules []gitIgnoreRule
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rule, err := compilePattern(line)
		if err != nil {
			log.Printf("Error compiling gitignore pattern %q: %v", line, err)
			continue
		}
		rules = append(rules, *rule)
	}
	return &GitIgnore{rules: rules}, scanner.Err()
}

// compilePattern converts a gitignore pattern to a compiled Regexp rule.
func compilePattern(pattern string) (*gitIgnoreRule, error) {
	negate := false
	if strings.HasPrefix(pattern, "!") {
		negate = true
		pattern = pattern[1:]
	}

	// Trim trailing slash but remember it only matches directories
	isDir := false
	if strings.HasSuffix(pattern, "/") {
		isDir = true
		pattern = strings.TrimSuffix(pattern, "/")
	}

	// If pattern contains no slash (except trailing), it matches anywhere.
	// Otherwise it is relative to the root.
	hasSlash := strings.Contains(pattern, "/")

	var sb strings.Builder
	if hasSlash {
		if strings.HasPrefix(pattern, "/") {
			sb.WriteString("^")
			pattern = pattern[1:]
		} else {
			sb.WriteString("^")
		}
	} else {
		sb.WriteString("(^|.*/)")
	}

	i := 0
	n := len(pattern)
	for i < n {
		char := pattern[i]
		switch char {
		case '*':
			if i+1 < n && pattern[i+1] == '*' {
				sb.WriteString(".*")
				i += 2
				if i < n && pattern[i] == '/' {
					i++
				}
			} else {
				sb.WriteString("[^/]*")
				i++
			}
		case '?':
			sb.WriteString("[^/]")
			i++
		case '\\', '.', '+', '$', '^', '(', ')', '[', ']', '{', '}', '|':
			sb.WriteByte('\\')
			sb.WriteByte(char)
			i++
		default:
			sb.WriteByte(char)
			i++
		}
	}

	sb.WriteString("($|/.*)")

	re, err := regexp.Compile(sb.String())
	if err != nil {
		return nil, err
	}

	return &gitIgnoreRule{
		regex:  re,
		negate: negate,
		isDir:  isDir,
	}, nil
}

// Match checks if the given relative path matches any pattern in the .gitignore.
func (gi *GitIgnore) Match(relPath string, isDir bool) bool {
	relPath = filepath.ToSlash(filepath.Clean(relPath))
	if relPath == "." || relPath == "/" {
		return false
	}
	relPathTrimmed := strings.TrimPrefix(relPath, "/")

	ignored := false
	for _, rule := range gi.rules {
		if rule.isDir && !isDir {
			continue
		}
		if rule.regex.MatchString(relPathTrimmed) {
			ignored = !rule.negate
		}
	}
	return ignored
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
	// Check if the target is a directory to pass to GitIgnore.Match
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
