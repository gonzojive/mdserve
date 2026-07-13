// Package gitignore provides a .gitignore parser and pattern matcher using a popular GitHub library.
package gitignore

import (
	"os"

	ignore "github.com/sabhiram/go-gitignore"
)

// GitIgnore holds patterns parsed and compiled from a .gitignore file.
type GitIgnore struct {
	impl *ignore.GitIgnore
}

// Load reads and parses a .gitignore file if it exists at the given path.
func Load(path string) (*GitIgnore, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &GitIgnore{}, nil
	}
	impl, err := ignore.CompileIgnoreFile(path)
	if err != nil {
		return nil, err
	}
	return &GitIgnore{impl: impl}, nil
}

// Match checks if the given relative path matches any pattern in the .gitignore.
// isDir is kept for compatibility with the project filtering interface.
func (gi *GitIgnore) Match(relPath string, isDir bool) bool {
	if gi.impl == nil {
		return false
	}
	return gi.impl.MatchesPath(relPath)
}
