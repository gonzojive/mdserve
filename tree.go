package main

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileNode represents a directory or file node in the workspace explorer.
type FileNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	IsDir    bool        `json:"isDir"`
	Size     int64       `json:"size"`
	SizeStr  string      `json:"sizeStr,omitempty"`
	Children []*FileNode `json:"children,omitempty"`
}

// PageData represents the variables passed to render the main HTML template.
type PageData struct {
	Title        string
	Content      template.HTML
	RawContent   string
	FileType     string // "markdown", "text", "image", "binary"
	FileSize     string
	FileTreeJSON template.JS
	CurrentPath  string
	Breadcrumbs  []Breadcrumb
	IsDirView    bool
	DirItems     []DirItem
	ShowAllFiles bool
}

// Breadcrumb represents a single breadcrumb element in the UI navigation bar.
type Breadcrumb struct {
	Name string
	Path string
}

// DirItem represents a single item inside a directory listing.
type DirItem struct {
	Name  string
	Path  string
	IsDir bool
	Size  string
}

// formatSize formats a byte count into a human-readable size string (e.g. 10.5 KB).
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// buildFileTree recursively crawls the target folder to construct a tree of directories and files.
func buildFileTree(rootDir string, showAll bool) (*FileNode, error) {
	var walk func(dir string) ([]*FileNode, error)
	walk = func(dir string) ([]*FileNode, error) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		var nodes []*FileNode
		for _, entry := range entries {
			name := entry.Name()
			if name == ".git" {
				continue
			}
			fullPath := filepath.Join(dir, name)
			relPath, err := filepath.Rel(rootDir, fullPath)
			if err != nil {
				continue
			}
			relPath = "/" + filepath.ToSlash(relPath)

			info, err := entry.Info()
			if err != nil {
				continue
			}

			node := &FileNode{
				Name:  name,
				Path:  relPath,
				IsDir: entry.IsDir(),
				Size:  info.Size(),
			}
			if entry.IsDir() {
				children, err := walk(fullPath)
				if err == nil && len(children) > 0 {
					node.Children = children
					nodes = append(nodes, node)
				}
			} else {
				ext := strings.ToLower(filepath.Ext(name))
				isMD := ext == ".md"
				if showAll || isMD {
					node.SizeStr = formatSize(info.Size())
					nodes = append(nodes, node)
				}
			}
		}

		// Sort: folders first, then files alphabetically
		sort.Slice(nodes, func(i, j int) bool {
			if nodes[i].IsDir != nodes[j].IsDir {
				return nodes[i].IsDir
			}
			return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
		})

		return nodes, nil
	}

	children, err := walk(rootDir)
	if err != nil {
		return nil, err
	}
	return &FileNode{
		Name:     filepath.Base(rootDir),
		Path:     "/",
		IsDir:    true,
		Children: children,
	}, nil
}

// makeBreadcrumbs converts a relative path into a slice of Breadcrumb navigation elements.
func makeBreadcrumbs(relPath string) []Breadcrumb {
	parts := strings.Split(strings.Trim(relPath, "/"), "/")
	var breadcrumbs []Breadcrumb
	breadcrumbs = append(breadcrumbs, Breadcrumb{Name: "Root", Path: "/"})
	if relPath == "" || relPath == "/" {
		return breadcrumbs
	}

	curr := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		curr += "/" + part
		breadcrumbs = append(breadcrumbs, Breadcrumb{Name: part, Path: curr})
	}
	return breadcrumbs
}
