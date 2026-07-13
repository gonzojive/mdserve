package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/parser"
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

// serveFile renders a single file to the browser, handling markdown rendering,
// syntax highlighting, image preview, or binary download options.
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

	if isImageFile(ext) {
		renderImage(&data, relPath)
	} else if info.Size() > 2*1024*1024 {
		renderLargeFile(&data, fileSizeStr, relPath)
	} else {
		content, err := os.ReadFile(filePath)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error reading file: %v", err), http.StatusInternalServerError)
			return
		}
		if isBinary(content) {
			renderBinaryFile(&data, fileSizeStr, relPath)
		} else if ext == ".md" {
			if err := renderMarkdownFile(&data, content, filePath, mdParser); err != nil {
				http.Error(w, fmt.Sprintf("Error parsing markdown: %v", err), http.StatusInternalServerError)
				return
			}
		} else {
			renderPlaintextFile(&data, content, ext)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := mainTemplate.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}

// isImageFile returns true if the extension belongs to a supported image type.
func isImageFile(ext string) bool {
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
	return imgExts[ext]
}

// renderImage configures the PageData for image rendering.
func renderImage(data *PageData, relPath string) {
	data.FileType = "image"
	data.Content = template.HTML(fmt.Sprintf(`<div class="image-viewer" style="text-align: center; padding: 20px;">
		<img src="%s?raw=true" style="max-width: 100%%; height: auto; border: 1px solid var(--border-color); border-radius: var(--radius-md); box-shadow: var(--shadow-md);" />
	</div>`, filepath.ToSlash(relPath)))
}

// renderLargeFile configures the PageData for files that exceed the 2MB size limit.
func renderLargeFile(data *PageData, fileSizeStr, relPath string) {
	data.FileType = "binary"
	data.Content = template.HTML(fmt.Sprintf(`<div class="binary-viewer" style="text-align: center; padding: 48px 0; border: 1px solid var(--border-color); border-radius: var(--radius-md); background: var(--bg-secondary);">
		<svg style="width: 64px; height: 64px; color: var(--text-secondary); margin-bottom: 16px;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"></path></svg>
		<h3 style="font-size: 18px; font-weight: 600; margin-bottom: 8px;">File is too large to render</h3>
		<p style="color: var(--text-secondary); margin-bottom: 16px;">Size: %s (Limit: 2MB)</p>
		<a href="%s?raw=true" class="theme-btn" style="display: inline-flex; align-items: center; gap: 8px; text-decoration: none; background: var(--accent-color); color: white; padding: 8px 16px;">
			Download File
		</a>
	</div>`, fileSizeStr, filepath.ToSlash(relPath)))
}

// renderBinaryFile configures the PageData for binary files.
func renderBinaryFile(data *PageData, fileSizeStr, relPath string) {
	data.FileType = "binary"
	data.Content = template.HTML(fmt.Sprintf(`<div class="binary-viewer" style="text-align: center; padding: 48px 0; border: 1px solid var(--border-color); border-radius: var(--radius-md); background: var(--bg-secondary);">
		<svg style="width: 64px; height: 64px; color: var(--text-secondary); margin-bottom: 16px;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5.586a1 1 0 0 1 .707.293l5.414 5.414a1 1 0 0 1 .293.707V19a2 2 0 0 1-2 2z"></path></svg>
		<h3 style="font-size: 18px; font-weight: 600; margin-bottom: 8px;">Binary File</h3>
		<p style="color: var(--text-secondary); margin-bottom: 16px;">Size: %s</p>
		<a href="%s?raw=true" class="theme-btn" style="display: inline-flex; align-items: center; gap: 8px; text-decoration: none; background: var(--accent-color); color: white; padding: 8px 16px;">
			Download File
		</a>
	</div>`, fileSizeStr, filepath.ToSlash(relPath)))
}

// renderMarkdownFile parses and registers markdown content within the PageData.
func renderMarkdownFile(data *PageData, content []byte, filePath string, mdParser goldmark.Markdown) error {
	data.RawContent = string(content)
	data.FileType = "markdown"

	var buf bytes.Buffer
	context := parser.NewContext()
	if err := mdParser.Convert(content, &buf, parser.WithContext(context)); err != nil {
		return err
	}
	data.Content = template.HTML(buf.String())

	// Extract title from front matter if available
	title := filepath.Base(filePath)
	metaData := meta.Get(context)
	if metaData != nil {
		if metaTitle, ok := metaData["title"].(string); ok {
			title = metaTitle
		}
	}
	data.Title = title
	return nil
}

// renderPlaintextFile highlights and wraps plain text code blocks in PageData.
func renderPlaintextFile(data *PageData, content []byte, ext string) {
	data.RawContent = string(content)
	data.FileType = "text"
	langClass := getLangClass(ext)
	escapedCode := template.HTMLEscapeString(data.RawContent)
	data.Content = template.HTML(fmt.Sprintf(`<pre><code class="language-%s" id="text-content-block">%s</code></pre>`, langClass, escapedCode))
}

// serveDirectory lists the items inside a folder.
func serveDirectory(w http.ResponseWriter, r *http.Request, dirPath, relPath string, rootDir string) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading directory: %v", err), http.StatusInternalServerError)
		return
	}

	var items []DirItem
	for _, entry := range entries {
		name := entry.Name()
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

	// Sort items: folders first, then files alphabetically
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
	if err := mainTemplate.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}

// serve404 returns a custom 404 HTML response with tree and breadcrumbs.
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
	if err := mainTemplate.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}
