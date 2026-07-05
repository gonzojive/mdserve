package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	htmlrenderer "github.com/yuin/goldmark/renderer/html"
)

// Hub maintains the set of active SSE clients and broadcasts file-change notifications.
type Hub struct {
	clients    map[chan string]bool
	register   chan chan string
	unregister chan chan string
	broadcast  chan string
	mu         sync.Mutex
}

func newHub() *Hub {
	return &Hub{
		clients:    make(map[chan string]bool),
		register:   make(chan chan string),
		unregister: make(chan chan string),
		broadcast:  make(chan string),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client)
			}
			h.mu.Unlock()
		case message := <-h.broadcast:
			h.mu.Lock()
			for client := range h.clients {
				select {
				case client <- message:
				default:
					close(client)
					delete(h.clients, client)
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) serveSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	messageChan := make(chan string)
	h.register <- messageChan

	defer func() {
		h.unregister <- messageChan
	}()

	// Send an initial handshake/keepalive event
	fmt.Fprintf(w, "data: connected\n\n")
	flusher.Flush()

	// Check if connection is closed
	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case msg := <-messageChan:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

// Watcher monitors the directory for changes.
type Watcher struct {
	watcher *fsnotify.Watcher
	dir     string
	hub     *Hub
}

func newWatcher(dir string, hub *Hub) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		watcher: w,
		dir:     dir,
		hub:     hub,
	}, nil
}

func (w *Watcher) watch() {
	// Walk directories and watch them recursively
	w.watchDir(w.dir)

	// Debounce file events to prevent double reloading
	var (
		mu         sync.Mutex
		delayTimer *time.Timer
	)

	triggerReload := func() {
		mu.Lock()
		defer mu.Unlock()
		if delayTimer != nil {
			delayTimer.Stop()
		}
		delayTimer = time.AfterFunc(200*time.Millisecond, func() {
			log.Println("Changes detected. Broadcasting reload...")
			w.hub.broadcast <- "reload"
		})
	}

	go func() {
		for {
			select {
			case event, ok := <-w.watcher.Events:
				if !ok {
					return
				}
				// Watch for markdown modifications or asset changes
				ext := strings.ToLower(filepath.Ext(event.Name))
				isMarkdown := ext == ".md"
				isAsset := ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".svg" || ext == ".css" || ext == ".js"

				if isMarkdown || isAsset {
					if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
						triggerReload()
					}
				}

				// Watch new subdirectories recursively
				if event.Op&fsnotify.Create != 0 {
					info, err := os.Stat(event.Name)
					if err == nil && info.IsDir() {
						log.Printf("New directory detected, adding to watch list: %s", event.Name)
						w.watcher.Add(event.Name)
					}
				}

			case err, ok := <-w.watcher.Errors:
				if !ok {
					return
				}
				log.Println("Watcher error:", err)
			}
		}
	}()
}

func (w *Watcher) watchDir(path string) {
	filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			err = w.watcher.Add(p)
			if err != nil {
				log.Printf("Error watching dir %s: %v", p, err)
			} else {
				log.Printf("Watching dir: %s", p)
			}
		}
		return nil
	})
}

// FileNode represents a directory or file node in the explorer.
type FileNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	IsDir    bool        `json:"isDir"`
	Children []*FileNode `json:"children,omitempty"`
}

// buildFileTree recursively crawls the target folder to construct a tree of directories and markdown files.
func buildFileTree(rootDir string) (*FileNode, error) {
	var walk func(dir string) ([]*FileNode, error)
	walk = func(dir string) ([]*FileNode, error) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		var nodes []*FileNode
		for _, entry := range entries {
			name := entry.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			fullPath := filepath.Join(dir, name)
			relPath, err := filepath.Rel(rootDir, fullPath)
			if err != nil {
				continue
			}
			relPath = "/" + filepath.ToSlash(relPath)

			node := &FileNode{
				Name:  name,
				Path:  relPath,
				IsDir: entry.IsDir(),
			}

			if entry.IsDir() {
				children, err := walk(fullPath)
				if err == nil {
					node.Children = children
				}
			} else {
				// Only include Markdown files in the tree navigation
				if strings.ToLower(filepath.Ext(name)) != ".md" {
					continue
				}
			}
			nodes = append(nodes, node)
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

// PageData represents the variables passed to the HTML template.
type PageData struct {
	Title        string
	Content      template.HTML
	FileTreeJSON template.JS
	CurrentPath  string
	Breadcrumbs  []Breadcrumb
	IsDirView    bool
	DirItems     []DirItem
}

type Breadcrumb struct {
	Name string
	Path string
}

type DirItem struct {
	Name  string
	Path  string
	IsDir bool
}

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

func main() {
	port := flag.Int("port", 8080, "Port to run server on")
	dirFlag := flag.String("dir", ".", "Directory of Markdown files to serve")
	flag.Parse()

	// Resolve absolute path of directory to serve
	targetDir, err := filepath.Abs(*dirFlag)
	if err != nil {
		log.Fatalf("Error resolving path: %v", err)
	}

	log.Printf("Starting Markdown Server in: %s", targetDir)

	// Create and start event hub
	hub := newHub()
	go hub.run()

	// Create and start directory watcher
	watcher, err := newWatcher(targetDir, hub)
	if err != nil {
		log.Fatalf("Error starting watcher: %v", err)
	}
	defer watcher.watcher.Close()
	watcher.watch()

	// Create markdown parser
	mdParser := goldmark.New(
		goldmark.WithExtensions(
			meta.Meta,
			extension.GFM,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			htmlrenderer.WithUnsafe(),
		),
	)

	// HTTP Handler
	http.HandleFunc("/events", hub.serveSSE)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		urlPath := r.URL.Path

		// Clean path to prevent path traversal
		cleanPath := filepath.Clean(urlPath)
		if strings.HasPrefix(cleanPath, "..") {
			http.Error(w, "Access Denied", http.StatusForbidden)
			return
		}

		localPath := filepath.Join(targetDir, cleanPath)

		// Check if file or directory exists
		info, err := os.Stat(localPath)
		if os.IsNotExist(err) {
			// If not exists, try appending .md (extensionless URLs)
			if !strings.HasSuffix(strings.ToLower(localPath), ".md") {
				mdPath := localPath + ".md"
				mdInfo, err := os.Stat(mdPath)
				if err == nil && !mdInfo.IsDir() {
					localPath = mdPath
					info = mdInfo
					cleanPath = cleanPath + ".md"
				} else {
					serve404(w, r, cleanPath, targetDir)
					return
				}
			} else {
				serve404(w, r, cleanPath, targetDir)
				return
			}
		}

		// Handle directory listing or auto-render readme
		if info.IsDir() {
			// Check for README.md, readme.md, index.md inside directory
			readmePath := ""
			readmes := []string{"README.md", "readme.md", "index.md", "README.MD", "INDEX.md"}
			for _, name := range readmes {
				p := filepath.Join(localPath, name)
				if rInfo, err := os.Stat(p); err == nil && !rInfo.IsDir() {
					readmePath = p
					cleanPath = filepath.Join(cleanPath, name)
					break
				}
			}

			if readmePath != "" {
				// Render readme markdown
				serveMarkdown(w, r, readmePath, cleanPath, targetDir, mdParser)
			} else {
				// Serve directory index
				serveDirectory(w, r, localPath, cleanPath, targetDir)
			}
			return
		}

		// Serve markdown or static assets
		ext := strings.ToLower(filepath.Ext(localPath))
		if ext == ".md" {
			serveMarkdown(w, r, localPath, cleanPath, targetDir, mdParser)
		} else {
			// Serve static asset (image, styles, etc.)
			http.ServeFile(w, r, localPath)
		}
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Server listening at http://localhost%s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func serveMarkdown(w http.ResponseWriter, r *http.Request, filePath, relPath string, rootDir string, mdParser goldmark.Markdown) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading file: %v", err), http.StatusInternalServerError)
		return
	}

	// Parse Markdown
	var buf bytes.Buffer
	context := parser.NewContext()
	if err := mdParser.Convert(content, &buf, parser.WithContext(context)); err != nil {
		http.Error(w, fmt.Sprintf("Error parsing markdown: %v", err), http.StatusInternalServerError)
		return
	}

	// Extracted title
	title := filepath.Base(filePath)
	metaData := meta.Get(context)
	if metaData != nil {
		if metaTitle, ok := metaData["title"].(string); ok {
			title = metaTitle
		}
	}

	// Build Sidebar File Tree
	tree, err := buildFileTree(rootDir)
	if err != nil {
		log.Printf("Error building file tree: %v", err)
	}
	treeJSON, _ := json.Marshal(tree)

	// Format page data
	data := PageData{
		Title:        title,
		Content:      template.HTML(buf.String()),
		FileTreeJSON: template.JS(treeJSON),
		CurrentPath:  filepath.ToSlash(relPath),
		Breadcrumbs:  makeBreadcrumbs(relPath),
		IsDirView:    false,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err = mainTemplate.Execute(w, data)
	if err != nil {
		log.Printf("Template execution error: %v", err)
	}
}

func serveDirectory(w http.ResponseWriter, r *http.Request, dirPath, relPath string, rootDir string) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading directory: %v", err), http.StatusInternalServerError)
		return
	}

	var items []DirItem
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		itemPath := filepath.Join(relPath, name)
		if entry.IsDir() {
			items = append(items, DirItem{Name: name, Path: filepath.ToSlash(itemPath), IsDir: true})
		} else if strings.ToLower(filepath.Ext(name)) == ".md" {
			items = append(items, DirItem{Name: name, Path: filepath.ToSlash(itemPath), IsDir: false})
		}
	}

	// Sort items
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	tree, err := buildFileTree(rootDir)
	if err != nil {
		log.Printf("Error building file tree: %v", err)
	}
	treeJSON, _ := json.Marshal(tree)

	data := PageData{
		Title:        filepath.Base(dirPath),
		Content:      "",
		FileTreeJSON: template.JS(treeJSON),
		CurrentPath:  filepath.ToSlash(relPath),
		Breadcrumbs:  makeBreadcrumbs(relPath),
		IsDirView:    true,
		DirItems:     items,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err = mainTemplate.Execute(w, data)
	if err != nil {
		log.Printf("Template execution error: %v", err)
	}
}

func serve404(w http.ResponseWriter, r *http.Request, relPath string, rootDir string) {
	w.WriteHeader(http.StatusNotFound)

	tree, err := buildFileTree(rootDir)
	if err != nil {
		log.Printf("Error building file tree: %v", err)
	}
	treeJSON, _ := json.Marshal(tree)

	content := `<div style="text-align: center; padding: 48px 0;">
		<svg style="width: 80px; height: 80px; color: var(--text-secondary); margin-bottom: 24px;" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.172 16.172a4 4 0 015.656 0M9 10h.01M15 10h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
		<h1 style="font-family: var(--font-title); font-size: 32px; font-weight: 700; margin-bottom: 16px;">404 - Page Not Found</h1>
		<p style="color: var(--text-secondary); margin-bottom: 24px;">The page you are looking for does not exist or has been moved.</p>
		<a href="/" style="display: inline-block; background-color: var(--accent-color); color: white; padding: 10px 20px; border-radius: var(--radius-md); text-decoration: none; font-weight: 500; transition: background-color 0.2s;">Go back home</a>
	</div>`

	data := PageData{
		Title:        "404 Not Found",
		Content:      template.HTML(content),
		FileTreeJSON: template.JS(treeJSON),
		CurrentPath:  filepath.ToSlash(relPath),
		Breadcrumbs:  makeBreadcrumbs(relPath),
		IsDirView:    false,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err = mainTemplate.Execute(w, data)
	if err != nil {
		log.Printf("Template execution error: %v", err)
	}
}

var mainTemplate = template.Must(template.New("main").Parse(`<!DOCTYPE html>
<html lang="en" data-color-mode="light">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - MDServe</title>
    <!-- Fonts -->
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=Outfit:wght@500;600;700&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
    
    <!-- GitHub Markdown CSS -->
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/github-markdown-css@5.5.1/github-markdown.min.css">
    
    <!-- Highlight.js CSS -->
    <link id="hljs-light" rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/github.min.css">
    <link id="hljs-dark" rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/github-dark.min.css" disabled>

    <style>
      :root {
        --bg-primary: #ffffff;
        --bg-secondary: #f6f8fa;
        --border-color: #d0d7de;
        --text-primary: #24292f;
        --text-secondary: #57606a;
        --accent-color: #0969da;
        --accent-hover: #0c57c2;
        --sidebar-width: 280px;
        --toc-width: 240px;
        --header-height: 56px;
        --shadow-sm: 0 1px 2px 0 rgba(0, 0, 0, 0.05);
        --shadow-md: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);
        --radius-md: 8px;
        --font-sans: 'Inter', -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
        --font-title: 'Outfit', var(--font-sans);
        --hover-bg: #eaeef2;
      }

      [data-color-mode="dark"] {
        --bg-primary: #0d1117;
        --bg-secondary: #161b22;
        --border-color: #30363d;
        --text-primary: #c9d1d9;
        --text-secondary: #8b949e;
        --accent-color: #58a6ff;
        --accent-hover: #79c0ff;
        --shadow-sm: 0 1px 2px 0 rgba(0, 0, 0, 0.5);
        --shadow-md: 0 4px 6px -1px rgba(0, 0, 0, 0.3), 0 2px 4px -1px rgba(0, 0, 0, 0.2);
        --hover-bg: #21262d;
      }

      body {
        margin: 0;
        padding: 0;
        font-family: var(--font-sans);
        background-color: var(--bg-primary);
        color: var(--text-primary);
        display: flex;
        min-height: 100vh;
        transition: background-color 0.3s ease, color 0.3s ease;
      }

      /* Sidebar */
      .sidebar {
        width: var(--sidebar-width);
        background-color: var(--bg-secondary);
        border-right: 1px solid var(--border-color);
        position: fixed;
        top: 0;
        bottom: 0;
        left: 0;
        display: flex;
        flex-direction: column;
        z-index: 100;
        transition: background-color 0.3s ease, border-color 0.3s ease;
      }

      .sidebar-header {
        padding: 16px 20px;
        display: flex;
        flex-direction: column;
        gap: 8px;
        border-bottom: 1px solid var(--border-color);
      }

      .sidebar-top {
        display: flex;
        align-items: center;
        justify-content: space-between;
      }

      .sidebar-logo {
        font-family: var(--font-title);
        font-size: 20px;
        font-weight: 700;
        color: var(--text-primary);
        display: flex;
        align-items: center;
        gap: 8px;
        text-decoration: none;
      }

      .status-badge {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        font-size: 11px;
        font-weight: 600;
        color: var(--text-secondary);
      }

      .status-dot {
        width: 8px;
        height: 8px;
        border-radius: 50%;
        background-color: #8b949e;
      }

      .status-dot.connected {
        background-color: #2ea44f;
        box-shadow: 0 0 8px #2ea44f;
        animation: pulse 2s infinite;
      }

      .status-dot.disconnected {
        background-color: #cf222e;
      }

      @keyframes pulse {
        0% { transform: scale(0.95); box-shadow: 0 0 0 0 rgba(46, 164, 79, 0.7); }
        70% { transform: scale(1); box-shadow: 0 0 0 6px rgba(46, 164, 79, 0); }
        100% { transform: scale(0.95); box-shadow: 0 0 0 0 rgba(46, 164, 79, 0); }
      }

      .sidebar-search {
        padding: 12px 20px;
        border-bottom: 1px solid var(--border-color);
      }

      .search-input {
        width: 100%;
        padding: 8px 12px;
        border: 1px solid var(--border-color);
        border-radius: var(--radius-md);
        background-color: var(--bg-primary);
        color: var(--text-primary);
        font-size: 13px;
        box-sizing: border-box;
        outline: none;
        transition: border-color 0.2s, box-shadow 0.2s;
      }

      .search-input:focus {
        border-color: var(--accent-color);
        box-shadow: 0 0 0 3px rgba(9, 105, 218, 0.15);
      }

      .sidebar-content {
        flex-grow: 1;
        overflow-y: auto;
        padding: 16px 12px;
      }

      .sidebar-footer {
        padding: 12px 20px;
        border-top: 1px solid var(--border-color);
        display: flex;
        align-items: center;
        justify-content: space-between;
      }

      /* File tree */
      .file-tree {
        list-style: none;
        padding: 0;
        margin: 0;
      }

      .tree-node {
        margin: 2px 0;
      }

      .tree-item-wrapper {
        display: flex;
        align-items: center;
        border-radius: 6px;
        transition: background-color 0.15s;
      }

      .tree-item-wrapper:hover {
        background-color: var(--hover-bg);
      }

      .tree-item {
        display: flex;
        align-items: center;
        padding: 6px 4px;
        cursor: pointer;
        text-decoration: none;
        color: var(--text-primary);
        font-size: 13px;
        gap: 6px;
        user-select: none;
        flex-grow: 1;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }

      .tree-item.active {
        color: var(--accent-color);
        font-weight: 600;
      }

      .tree-item-wrapper.active {
        background-color: rgba(9, 105, 218, 0.08);
      }

      [data-color-mode="dark"] .tree-item-wrapper.active {
        background-color: rgba(88, 166, 255, 0.12);
      }

      .tree-arrow {
        width: 20px;
        height: 20px;
        display: flex;
        align-items: center;
        justify-content: center;
        cursor: pointer;
        border-radius: 4px;
        color: var(--text-secondary);
        transition: transform 0.15s ease;
      }

      .tree-arrow:hover {
        background-color: rgba(0,0,0,0.05);
      }

      [data-color-mode="dark"] .tree-arrow:hover {
        background-color: rgba(255,255,255,0.05);
      }

      .tree-arrow.collapsed {
        transform: rotate(-90deg);
      }

      .tree-arrow.leaf {
        visibility: hidden;
      }

      .tree-icon {
        flex-shrink: 0;
        width: 16px;
        height: 16px;
        color: var(--text-secondary);
      }

      .tree-item.active .tree-icon {
        color: var(--accent-color);
      }

      .tree-children {
        list-style: none;
        padding-left: 12px;
        margin: 0;
        display: none;
        border-left: 1px solid var(--border-color);
        margin-left: 9px;
      }

      .tree-children.expanded {
        display: block;
      }

      /* Theme Toggle Button */
      .theme-btn {
        background: none;
        border: 1px solid var(--border-color);
        border-radius: var(--radius-md);
        padding: 6px 10px;
        cursor: pointer;
        color: var(--text-primary);
        display: flex;
        align-items: center;
        justify-content: center;
        transition: background-color 0.2s, border-color 0.2s;
      }

      .theme-btn:hover {
        background-color: var(--hover-bg);
      }

      .theme-btn svg {
        width: 16px;
        height: 16px;
      }

      /* Main container layout */
      .main-wrapper {
        margin-left: var(--sidebar-width);
        margin-right: var(--toc-width);
        flex-grow: 1;
        padding: 40px 48px;
        display: flex;
        flex-direction: column;
        align-items: center;
        box-sizing: border-box;
      }

      .main-content {
        width: 100%;
        max-width: 860px;
        box-sizing: border-box;
      }

      /* Breadcrumbs */
      .breadcrumbs {
        font-size: 13px;
        color: var(--text-secondary);
        margin-bottom: 24px;
        display: flex;
        align-items: center;
        gap: 6px;
      }

      .breadcrumbs a {
        color: var(--text-secondary);
        text-decoration: none;
        transition: color 0.15s;
      }

      .breadcrumbs a:hover {
        color: var(--accent-color);
      }

      .breadcrumb-separator {
        color: var(--border-color);
        font-size: 12px;
      }

      /* Right Sidebar (TOC) */
      .toc-sidebar {
        width: var(--toc-width);
        position: fixed;
        top: 0;
        bottom: 0;
        right: 0;
        border-left: 1px solid var(--border-color);
        padding: 40px 20px;
        overflow-y: auto;
        box-sizing: border-box;
        background-color: var(--bg-primary);
        transition: background-color 0.3s ease, border-color 0.3s ease;
      }

      .toc-title {
        font-size: 11px;
        font-weight: 700;
        text-transform: uppercase;
        color: var(--text-secondary);
        margin-bottom: 12px;
        letter-spacing: 0.5px;
      }

      .toc-list {
        list-style: none;
        padding: 0;
        margin: 0;
      }

      .toc-item {
        margin: 8px 0;
      }

      .toc-link {
        font-size: 13px;
        color: var(--text-secondary);
        text-decoration: none;
        display: block;
        line-height: 1.4;
        transition: color 0.15s, border-left-color 0.15s;
        border-left: 2px solid transparent;
        padding-left: 8px;
      }

      .toc-link:hover {
        color: var(--text-primary);
      }

      .toc-link.active {
        color: var(--accent-color);
        font-weight: 500;
        border-left-color: var(--accent-color);
      }

      .toc-link.h2 { margin-left: 10px; }
      .toc-link.h3 { margin-left: 20px; }

      /* Markdown Body Style */
      .markdown-body {
        background-color: transparent !important;
        font-family: var(--font-sans) !important;
      }

      /* Mermaid styling */
      .mermaid {
        background-color: var(--bg-secondary) !important;
        border: 1px solid var(--border-color) !important;
        border-radius: var(--radius-md) !important;
        padding: 16px !important;
        margin: 16px 0 !important;
        display: flex;
        justify-content: center;
        overflow-x: auto;
      }

      /* Back to Top */
      .back-to-top {
        position: fixed;
        bottom: 24px;
        right: calc(var(--toc-width) + 24px);
        background-color: var(--bg-primary);
        border: 1px solid var(--border-color);
        border-radius: 50%;
        width: 40px;
        height: 40px;
        display: flex;
        align-items: center;
        justify-content: center;
        cursor: pointer;
        color: var(--text-secondary);
        box-shadow: var(--shadow-md);
        opacity: 0;
        visibility: hidden;
        transition: opacity 0.3s, visibility 0.3s, background-color 0.2s;
        z-index: 99;
      }

      .back-to-top.visible {
        opacity: 1;
        visibility: visible;
      }

      .back-to-top:hover {
        color: var(--text-primary);
        background-color: var(--hover-bg);
      }

      /* Directory card listing */
      .dir-title {
        font-family: var(--font-title);
        font-size: 24px;
        font-weight: 700;
        margin-top: 0;
        margin-bottom: 8px;
      }

      .dir-description {
        color: var(--text-secondary);
        font-size: 14px;
        margin-bottom: 24px;
      }

      .dir-grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
        gap: 16px;
        margin-top: 16px;
      }

      .dir-card {
        border: 1px solid var(--border-color);
        border-radius: var(--radius-md);
        padding: 20px 16px;
        text-decoration: none;
        color: var(--text-primary);
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 12px;
        background-color: var(--bg-secondary);
        transition: border-color 0.2s, transform 0.2s, box-shadow 0.2s;
      }

      .dir-card:hover {
        border-color: var(--accent-color);
        transform: translateY(-2px);
        box-shadow: var(--shadow-sm);
      }

      .dir-card-icon {
        width: 36px;
        height: 36px;
        color: var(--accent-color);
      }

      .dir-card-name {
        font-size: 13px;
        font-weight: 600;
        text-align: center;
        word-break: break-all;
      }

      /* Responsive */
      @media (max-width: 1024px) {
        .toc-sidebar {
          display: none;
        }
        .main-wrapper {
          margin-right: 0;
          padding: 24px;
        }
        .back-to-top {
          right: 24px;
        }
      }

      @media (max-width: 768px) {
        body {
          flex-direction: column;
        }
        .sidebar {
          position: relative;
          width: 100%;
          height: auto;
          border-right: none;
          border-bottom: 1px solid var(--border-color);
        }
        .sidebar-content {
          max-height: 250px;
        }
        .main-wrapper {
          margin-left: 0;
          padding: 20px;
        }
      }
    </style>
</head>
<body>

    <!-- Sidebar Explorer -->
    <aside class="sidebar">
        <div class="sidebar-header">
            <div class="sidebar-top">
                <a href="/" class="sidebar-logo">
                    <svg style="width: 24px; height: 24px; color: var(--accent-color);" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><line x1="16" y1="13" x2="8" y2="13"></line><line x1="16" y1="17" x2="8" y2="17"></line><polyline points="10 9 9 9 8 9"></polyline></svg>
                    <span>MDServe</span>
                </a>
                <span class="status-badge">
                    <span id="status-dot" class="status-dot disconnected"></span>
                </span>
            </div>
        </div>
        <div class="sidebar-search">
            <input type="text" id="search-box" class="search-input" placeholder="Search files...">
        </div>
        <div class="sidebar-content">
            <ul class="file-tree" id="file-tree-root"></ul>
        </div>
        <div class="sidebar-footer">
            <span style="font-size: 11px; color: var(--text-secondary);">v1.0.0</span>
            <button class="theme-btn" id="theme-toggle" title="Toggle theme">
                <!-- Sun Icon -->
                <svg id="theme-sun" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="display: none;"><circle cx="12" cy="12" r="5"></circle><line x1="12" y1="1" x2="12" y2="3"></line><line x1="12" y1="21" x2="12" y2="23"></line><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"></line><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"></line><line x1="1" y1="12" x2="3" y2="12"></line><line x1="21" y1="12" x2="23" y2="12"></line><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"></line><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"></line></svg>
                <!-- Moon Icon -->
                <svg id="theme-moon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"></path></svg>
            </button>
        </div>
    </aside>

    <!-- Main Content Wrapper -->
    <main class="main-wrapper">
        <div class="main-content">
            <!-- Breadcrumbs -->
            <nav class="breadcrumbs">
                {{range $i, $bc := .Breadcrumbs}}
                    {{if $i}}<span class="breadcrumb-separator">/</span>{{end}}
                    <a href="{{$bc.Path}}">{{$bc.Name}}</a>
                {{end}}
            </nav>

            {{if .IsDirView}}
                <!-- Directory listing view -->
                <h2 class="dir-title">Browsing: {{.Title}}</h2>
                <p class="dir-description">Select a folder or a Markdown file to view its rendering.</p>
                <div class="dir-grid">
                    {{range .DirItems}}
                        <a href="{{.Path}}" class="dir-card">
                            {{if .IsDir}}
                                <svg class="dir-card-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path></svg>
                            {{else}}
                                <svg class="dir-card-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><line x1="16" y1="13" x2="8" y2="13"></line><line x1="16" y1="17" x2="8" y2="17"></line><polyline points="10 9 9 9 8 9"></polyline></svg>
                            {{end}}
                            <div class="dir-card-name">{{.Name}}</div>
                        </a>
                    {{end}}
                </div>
            {{else}}
                <!-- Rendered Markdown view -->
                <article class="markdown-body">
                    {{.Content}}
                </article>
            {{end}}
        </div>
    </main>

    <!-- Table of Contents Sidebar -->
    <aside class="toc-sidebar">
        <div class="toc-title">On This Page</div>
        <ul class="toc-list" id="toc-list">
            <!-- Populated via JS -->
            <li class="toc-item" style="font-size: 13px; color: var(--text-secondary); font-style: italic;">No headings found</li>
        </ul>
    </aside>

    <!-- Back to Top Button -->
    <button class="back-to-top" id="back-to-top" title="Back to top">
        <svg style="width: 20px; height: 20px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="19" x2="12" y2="5"></line><polyline points="5 12 12 5 19 12"></polyline></svg>
    </button>

    <!-- Highlight.js JS -->
    <script src="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js"></script>

    <!-- JS Logic -->
    <script>
      // Load file tree from Go
      const fileTreeData = {{.FileTreeJSON}};
      const currentPath = {{.CurrentPath}};

      // Theme Management
      const themeToggleBtn = document.getElementById('theme-toggle');
      const themeSunIcon = document.getElementById('theme-sun');
      const themeMoonIcon = document.getElementById('theme-moon');
      const hljsLight = document.getElementById('hljs-light');
      const hljsDark = document.getElementById('hljs-dark');

      function setTheme(mode) {
        document.documentElement.setAttribute('data-color-mode', mode);
        localStorage.setItem('theme', mode);
        if (mode === 'dark') {
          themeSunIcon.style.display = 'block';
          themeMoonIcon.style.display = 'none';
          hljsLight.disabled = true;
          hljsDark.disabled = false;
        } else {
          themeSunIcon.style.display = 'none';
          themeMoonIcon.style.display = 'block';
          hljsLight.disabled = false;
          hljsDark.disabled = true;
        }
      }

      // Initialize theme from storage or media queries
      const savedTheme = localStorage.getItem('theme') || (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
      setTheme(savedTheme);

      themeToggleBtn.addEventListener('click', () => {
        const currentMode = document.documentElement.getAttribute('data-color-mode');
        setTheme(currentMode === 'dark' ? 'light' : 'dark');
      });

      // Render File Tree in Sidebar
      const fileTreeRoot = document.getElementById('file-tree-root');

      function renderTree(nodes, container, parentPath = '') {
        if (!nodes) return;
        nodes.forEach(node => {
          const li = document.createElement('li');
          li.className = 'tree-node';

          const itemWrapper = document.createElement('div');
          itemWrapper.className = 'tree-item-wrapper';
          if (currentPath === node.path) {
             itemWrapper.classList.add('active');
          }

          // Arrow indicator for dirs
          const arrow = document.createElement('div');
          arrow.className = 'tree-arrow';
          if (node.isDir) {
             arrow.innerHTML = '<svg style="width: 12px; height: 12px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="6 9 12 15 18 9"></polyline></svg>';
          } else {
             arrow.classList.add('leaf');
          }
          itemWrapper.appendChild(arrow);

          // Icon
          const icon = document.createElement('span');
          icon.className = 'tree-icon';
          if (node.isDir) {
             icon.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="width:100%;height:100%;"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path></svg>';
          } else {
             icon.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="width:100%;height:100%;"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><line x1="16" y1="13" x2="8" y2="13"></line><line x1="16" y1="17" x2="8" y2="17"></line></svg>';
          }
          itemWrapper.appendChild(icon);

          // Item Link
          const a = document.createElement('a');
          a.className = 'tree-item';
          a.textContent = node.name;
          a.href = node.path;
          if (currentPath === node.path) {
             a.classList.add('active');
          }
          itemWrapper.appendChild(a);

          li.appendChild(itemWrapper);

          // Children container (if dir)
          if (node.isDir && node.children && node.children.length > 0) {
             const subContainer = document.createElement('ul');
             subContainer.className = 'tree-children';
             li.appendChild(subContainer);
             renderTree(node.children, subContainer, node.path);

             // Handle collapse / expand logic
             const isParentOfCurrent = currentPath.startsWith(node.path + '/') || currentPath === node.path;
             if (isParentOfCurrent) {
                subContainer.classList.add('expanded');
             } else {
                arrow.classList.add('collapsed');
             }

             const toggleDir = () => {
                const isExpanded = subContainer.classList.toggle('expanded');
                arrow.classList.toggle('collapsed', !isExpanded);
             };

             arrow.addEventListener('click', (e) => {
                e.stopPropagation();
                toggleDir();
             });
             
             // Clicking folder row toggles folder unless it has an active link
             itemWrapper.addEventListener('click', (e) => {
                if (e.target !== a) {
                   e.preventDefault();
                   toggleDir();
                }
             });
          }

          container.appendChild(li);
        });
      }

      if (fileTreeData && fileTreeData.children) {
         renderTree(fileTreeData.children, fileTreeRoot);
      }

      // Search Box Filter
      const searchBox = document.getElementById('search-box');
      searchBox.addEventListener('input', (e) => {
        const query = e.target.value.toLowerCase().trim();
        const treeItems = fileTreeRoot.querySelectorAll('.tree-node');

        if (query === '') {
           // Reset tree view back to normal
           treeItems.forEach(node => {
              node.style.display = '';
              const children = node.querySelector('.tree-children');
              const arrow = node.querySelector('.tree-arrow');
              if (children && arrow) {
                 const currentHref = node.querySelector('.tree-item').getAttribute('href');
                 const hasActiveChild = currentPath.startsWith(currentHref + '/');
                 if (hasActiveChild || currentPath === currentHref) {
                    children.classList.add('expanded');
                    arrow.classList.remove('collapsed');
                 } else {
                    children.classList.remove('expanded');
                    arrow.classList.add('collapsed');
                 }
              }
           });
           return;
        }

        // Search mode: show all nodes matching, and force parent expansion
        treeItems.forEach(node => {
           const itemName = node.querySelector('.tree-item').textContent.toLowerCase();
           const match = itemName.includes(query);
           
           if (match) {
              node.style.display = '';
              // Expand all parents of matching node
              let parent = node.parentElement;
              while (parent && parent !== fileTreeRoot) {
                 if (parent.classList.contains('tree-children')) {
                    parent.classList.add('expanded');
                    const arrow = parent.previousElementSibling.querySelector('.tree-arrow');
                    if (arrow) arrow.classList.remove('collapsed');
                 }
                 if (parent.classList.contains('tree-node')) {
                    parent.style.display = '';
                 }
                 parent = parent.parentElement;
              }
           } else {
              node.style.display = 'none';
           }
        });
      });

      // Extract Headings for TOC & ScrollSpy
      const markdownBody = document.querySelector('.markdown-body');
      const tocList = document.getElementById('toc-list');
      
      if (markdownBody) {
         const headings = Array.from(markdownBody.querySelectorAll('h1, h2, h3'));
         if (headings.length > 0) {
            tocList.innerHTML = ''; // Clear empty state
            
            headings.forEach(heading => {
               // Ensure heading has an ID
               if (!heading.id) {
                  heading.id = heading.textContent.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '');
               }
               
               const li = document.createElement('li');
               li.className = 'toc-item';
               
               const a = document.createElement('a');
               a.className = 'toc-link ' + heading.tagName.toLowerCase();
               a.textContent = heading.textContent;
               a.href = '#' + heading.id;
               
               a.addEventListener('click', (e) => {
                  e.preventDefault();
                  heading.scrollIntoView({ behavior: 'smooth' });
                  history.pushState(null, null, '#' + heading.id);
               });
               
               li.appendChild(a);
               tocList.appendChild(li);
            });

            // ScrollSpy
            const tocLinks = tocList.querySelectorAll('.toc-link');
            const scrollSpy = () => {
               const scrollPosition = window.scrollY + 100;
               
               let activeHeading = null;
               for (let i = 0; i < headings.length; i++) {
                  if (headings[i].offsetTop <= scrollPosition) {
                     activeHeading = headings[i];
                  } else {
                     break;
                  }
               }
               
               tocLinks.forEach(link => {
                  link.classList.remove('active');
                  if (activeHeading && link.getAttribute('href') === '#' + activeHeading.id) {
                     link.classList.add('active');
                  }
               });
            };
            
            window.addEventListener('scroll', scrollSpy);
            // Run once initially
            scrollSpy();
         }
      }

      // Back to Top Button
      const backToTopBtn = document.getElementById('back-to-top');
      if (backToTopBtn) {
         window.addEventListener('scroll', () => {
            if (window.scrollY > 300) {
               backToTopBtn.classList.add('visible');
            } else {
               backToTopBtn.classList.remove('visible');
            }
         });
         backToTopBtn.addEventListener('click', () => {
            window.scrollTo({ top: 0, behavior: 'smooth' });
         });
      }

      // Highlight JS syntax coloring
      hljs.highlightAll();

      // Convert Markdown Mermaid blocks for client-side Mermaid rendering
      document.querySelectorAll('pre code.language-mermaid').forEach((block) => {
         const pre = block.parentNode;
         const container = document.createElement('pre');
         container.className = 'mermaid';
         container.textContent = block.textContent.trim();
         pre.replaceWith(container);
      });

      // Dynamic Mermaid ESM Import
      import('https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs').then((module) => {
         const mermaid = module.default;
         const isDark = document.documentElement.getAttribute('data-color-mode') === 'dark';
         mermaid.initialize({ 
            startOnLoad: true,
            theme: isDark ? 'dark' : 'default',
            securityLevel: 'loose'
         });
      }).catch(err => console.error("Mermaid script loading error:", err));

      // SSE Live Reload Connection
      const statusDot = document.getElementById('status-dot');
      
      function connectSSE() {
         const eventSource = new EventSource('/events');
         
         eventSource.onopen = function() {
            statusDot.className = 'status-dot connected';
            statusDot.title = 'Live reload connected';
         };
         
         eventSource.onmessage = function(event) {
            if (event.data === 'reload') {
               console.log('File change detected. Reloading page...');
               location.reload();
            }
         };
         
         eventSource.onerror = function() {
            statusDot.className = 'status-dot disconnected';
            statusDot.title = 'Live reload disconnected, attempting reconnection...';
            eventSource.close();
            // Reconnect after 3 seconds
            setTimeout(connectSSE, 3000);
         };
      }
      
      connectSSE();
    </script>
</body>
</html>`))
