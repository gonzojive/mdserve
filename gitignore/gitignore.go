// Package gitignore provides a zero-dependency .gitignore parser and pattern matcher.
package gitignore

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type gitIgnoreRule struct {
	regex  *regexp.Regexp
	negate bool
	isDir  bool
}

// GitIgnore holds patterns parsed and compiled from a .gitignore file.
type GitIgnore struct {
	rules []gitIgnoreRule
}

// Load reads and parses a .gitignore file if it exists at the given path.
func Load(path string) (*GitIgnore, error) {
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
