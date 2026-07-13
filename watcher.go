package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher monitors the directory tree recursively for changes and
// triggers reload events on the event hub.
type Watcher struct {
	watcher *fsnotify.Watcher
	dir     string
	hub     *Hub
}

// newWatcher creates a new Watcher configured for the target directory
// and event hub.
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

// watch walks directories recursively, adds them to the watch list,
// and starts the asynchronous file event monitoring loop.
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

// watchDir recursively crawls a path adding every directory to the fsnotify watch list.
func (w *Watcher) watchDir(path string) {
	err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			err = w.watcher.Add(p)
			if err != nil {
				log.Printf("Error watching dir %s: %v", p, err)
			} else {
				log.Printf("Watching dir: %s", p)
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("Error walking directories for watching: %v", err)
	}
}
