package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
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

//go:embed templates/main.html
var mainTemplateRaw string

//go:embed third_party/*
var thirdPartyFS embed.FS

var mainTemplate *template.Template

func init() {
	mainTemplate = template.Must(template.New("main").Parse(mainTemplateRaw))
}

// ShowAllFiles controls whether we show and serve all files in the directory.
var ShowAllFiles bool

// formatSize formats a byte count into a human-readable size string.
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

// isBinary detects if content is binary by checking for null bytes in the first 1024 bytes.
func isBinary(content []byte) bool {
	limit := len(content)
	if limit > 1024 {
		limit = 1024
	}
	for i := 0; i < limit; i++ {
		if content[i] == 0 {
			return true
		}
	}
	return false
}

// getLangClass maps file extensions to highlight.js language classes.
func getLangClass(ext string) string {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	switch ext {
	case "go":
		return "go"
	case "js", "jsx":
		return "javascript"
	case "ts", "tsx":
		return "typescript"
	case "css":
		return "css"
	case "html", "htm", "xml":
		return "xml"
	case "json":
		return "json"
	case "yaml", "yml":
		return "yaml"
	case "sh", "bash":
		return "bash"
	case "py":
		return "python"
	case "md":
		return "markdown"
	case "sql":
		return "sql"
	case "dockerfile":
		return "dockerfile"
	case "ini", "toml":
		return "ini"
	default:
		return "plaintext"
	}
}

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
	Size     int64       `json:"size"`
	SizeStr  string      `json:"sizeStr,omitempty"`
	Children []*FileNode `json:"children,omitempty"`
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
			if strings.HasPrefix(name, ".") {
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

// PageData represents the variables passed to the HTML template.
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

type Breadcrumb struct {
	Name string
	Path string
}

type DirItem struct {
	Name  string
	Path  string
	IsDir bool
	Size  string
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
	subFS, err := fs.Sub(thirdPartyFS, "third_party")
	if err != nil {
		log.Fatalf("Error creating sub-filesystem for third_party: %v", err)
	}
	http.Handle("/third_party/", http.StripPrefix("/third_party/", http.FileServer(http.FS(subFS))))

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
				// Render readme file
				serveFile(w, r, readmePath, cleanPath, targetDir, mdParser)
			} else {
				// Serve directory index
				serveDirectory(w, r, localPath, cleanPath, targetDir)
			}
			return
		}

		// Serve file
		wantsHTML := strings.Contains(r.Header.Get("Accept"), "text/html")
		if !wantsHTML || r.URL.Query().Get("raw") == "true" {
			http.ServeFile(w, r, localPath)
			return
		}

		serveFile(w, r, localPath, cleanPath, targetDir, mdParser)
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Server listening at http://localhost%s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func serveFile(w http.ResponseWriter, r *http.Request, filePath, relPath string, rootDir string, mdParser goldmark.Markdown) {
	if r.URL.Query().Get("raw") == "true" {
		http.ServeFile(w, r, filePath)
		return
	}

	info, err := os.Stat(filePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading file info: %v", err), http.StatusInternalServerError)
		return
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	fileSizeStr := formatSize(info.Size())

	// Check if it's an image
	isImg := false
	imgExts := map[string]bool{
		".png":  true,
		".jpg":  true,
		".jpeg": true,
		".gif":  true,
		".svg":  true,
		".webp": true,
		".ico":  true,
		".bmp":  true,
		".tiff": true,
	}
	if imgExts[ext] {
		isImg = true
	}

	// Build Sidebar File Tree
	tree, err := buildFileTree(rootDir, ShowAllFiles)
	if err != nil {
		log.Printf("Error building file tree: %v", err)
	}
	treeJSON, _ := json.Marshal(tree)

	data := PageData{
		Title:        filepath.Base(filePath),
		FileSize:     fileSizeStr,
		FileTreeJSON: template.JS(treeJSON),
		CurrentPath:  filepath.ToSlash(relPath),
		Breadcrumbs:  makeBreadcrumbs(relPath),
		IsDirView:    false,
		ShowAllFiles: ShowAllFiles,
	}

	if isImg {
		data.FileType = "image"
		data.Content = template.HTML(fmt.Sprintf(`<div class="image-viewer" style="text-align: center; padding: 20px;">
			<img src="%s?raw=true" style="max-width: 100%%; height: auto; border: 1px solid var(--border-color); border-radius: var(--radius-md); box-shadow: var(--shadow-md);" />
		</div>`, filepath.ToSlash(relPath)))
	} else if info.Size() > 2*1024*1024 {
		// Too large to render (2MB limit)
		data.FileType = "binary"
		data.Content = template.HTML(fmt.Sprintf(`<div class="binary-viewer" style="text-align: center; padding: 48px 0; border: 1px solid var(--border-color); border-radius: var(--radius-md); background: var(--bg-secondary);">
			<svg style="width: 64px; height: 64px; color: var(--text-secondary); margin-bottom: 16px;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"></path></svg>
			<h3 style="font-size: 18px; font-weight: 600; margin-bottom: 8px;">File is too large to render</h3>
			<p style="color: var(--text-secondary); margin-bottom: 16px;">Size: %s</p>
			<a href="%s?raw=true" class="theme-btn" style="display: inline-flex; align-items: center; gap: 8px; text-decoration: none; background: var(--accent-color); color: white; padding: 8px 16px;">
				Download File
			</a>
		</div>`, fileSizeStr, filepath.ToSlash(relPath)))
	} else {
		// Read content
		content, err := os.ReadFile(filePath)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error reading file: %v", err), http.StatusInternalServerError)
			return
		}

		if isBinary(content) {
			data.FileType = "binary"
			data.Content = template.HTML(fmt.Sprintf(`<div class="binary-viewer" style="text-align: center; padding: 48px 0; border: 1px solid var(--border-color); border-radius: var(--radius-md); background: var(--bg-secondary);">
				<svg style="width: 64px; height: 64px; color: var(--text-secondary); margin-bottom: 16px;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5.586a1 1 0 0 1 .707.293l5.414 5.414a1 1 0 0 1 .293.707V19a2 2 0 0 1-2 2z"></path></svg>
				<h3 style="font-size: 18px; font-weight: 600; margin-bottom: 8px;">Binary File</h3>
				<p style="color: var(--text-secondary); margin-bottom: 16px;">Size: %s</p>
				<a href="%s?raw=true" class="theme-btn" style="display: inline-flex; align-items: center; gap: 8px; text-decoration: none; background: var(--accent-color); color: white; padding: 8px 16px;">
					Download File
				</a>
			</div>`, fileSizeStr, filepath.ToSlash(relPath)))
		} else {
			data.RawContent = string(content)
			if ext == ".md" {
				data.FileType = "markdown"

				// Parse Markdown
				var buf bytes.Buffer
				context := parser.NewContext()
				if err := mdParser.Convert(content, &buf, parser.WithContext(context)); err != nil {
					http.Error(w, fmt.Sprintf("Error parsing markdown: %v", err), http.StatusInternalServerError)
					return
				}
				data.Content = template.HTML(buf.String())

				// Extract title
				title := filepath.Base(filePath)
				metaData := meta.Get(context)
				if metaData != nil {
					if metaTitle, ok := metaData["title"].(string); ok {
						title = metaTitle
					}
				}
				data.Title = title
			} else {
				data.FileType = "text"
				langClass := getLangClass(ext)
				escapedCode := template.HTMLEscapeString(data.RawContent)
				data.Content = template.HTML(fmt.Sprintf(`<pre><code class="language-%s" id="text-content-block">%s</code></pre>`, langClass, escapedCode))
			}
		}
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
		
		info, err := entry.Info()
		if err != nil {
			continue
		}

		ext := strings.ToLower(filepath.Ext(name))
		isMD := ext == ".md"

		if entry.IsDir() {
			items = append(items, DirItem{Name: name, Path: filepath.ToSlash(itemPath), IsDir: true})
		} else if ShowAllFiles || isMD {
			items = append(items, DirItem{
				Name:  name,
				Path:  filepath.ToSlash(itemPath),
				IsDir: false,
				Size:  formatSize(info.Size()),
			})
		}
	}

	// Sort items
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	tree, err := buildFileTree(rootDir, ShowAllFiles)
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
		ShowAllFiles: ShowAllFiles,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err = mainTemplate.Execute(w, data)
	if err != nil {
		log.Printf("Template execution error: %v", err)
	}
}

func serve404(w http.ResponseWriter, r *http.Request, relPath string, rootDir string) {
	w.WriteHeader(http.StatusNotFound)

	tree, err := buildFileTree(rootDir, ShowAllFiles)
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
		ShowAllFiles: ShowAllFiles,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err = mainTemplate.Execute(w, data)
	if err != nil {
		log.Printf("Template execution error: %v", err)
	}
}

