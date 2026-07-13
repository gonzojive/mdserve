package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// runServer starts the event hub, directory watcher, registers the HTTP routes,
// and blocks while listening for connections on the specified port.
func runServer(port int, targetDir string, allFlag bool) error {
	ShowAllFiles = allFlag

	// Resolve absolute path of directory to serve
	absDir, err := filepath.Abs(targetDir)
	if err != nil {
		return fmt.Errorf("error resolving path: %w", err)
	}

	log.Printf("Starting Markdown Server in: %s", absDir)

	// Create and start event hub
	hub := newHub()
	go hub.run()

	// Create and start directory watcher
	watcher, err := newWatcher(absDir, hub)
	if err != nil {
		return fmt.Errorf("error starting watcher: %w", err)
	}
	defer watcher.watcher.Close()
	watcher.watch()

	// Create markdown parser
	mdParser := createMarkdownParser()

	// HTTP Handler Configuration
	mux := http.NewServeMux()
	mux.HandleFunc("/events", hub.serveSSE)

	subFS, err := fs.Sub(thirdPartyFS, "third_party")
	if err != nil {
		return fmt.Errorf("error creating sub-filesystem for third_party: %w", err)
	}
	mux.Handle("/third_party/", http.StripPrefix("/third_party/", http.FileServer(http.FS(subFS))))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		urlPath := r.URL.Path

		// Clean path to prevent path traversal
		cleanPath := filepath.Clean(urlPath)
		if strings.HasPrefix(cleanPath, "..") {
			http.Error(w, "Access Denied", http.StatusForbidden)
			return
		}

		localPath := filepath.Join(absDir, cleanPath)

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
					serve404(w, r, cleanPath, absDir)
					return
				}
			} else {
				serve404(w, r, cleanPath, absDir)
				return
			}
		}

		// Handle directory listing or auto-render readme
		if info.IsDir() {
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
				serveFile(w, r, readmePath, cleanPath, absDir, mdParser)
			} else {
				serveDirectory(w, r, localPath, cleanPath, absDir)
			}
			return
		}

		// Serve file
		wantsHTML := strings.Contains(r.Header.Get("Accept"), "text/html")
		if !wantsHTML || r.URL.Query().Get("raw") == "true" {
			http.ServeFile(w, r, localPath)
			return
		}

		serveFile(w, r, localPath, cleanPath, absDir, mdParser)
	})

	addr := fmt.Sprintf(":%d", port)
	log.Printf("Server listening at http://localhost%s", addr)
	return http.ListenAndServe(addr, mux)
}
