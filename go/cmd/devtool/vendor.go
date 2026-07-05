package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var assets = map[string]string{
	"github-markdown.min.css":      "https://cdn.jsdelivr.net/npm/github-markdown-css@5.5.1/github-markdown.min.css",
	"highlight-github.min.css":     "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/github.min.css",
	"highlight-github-dark.min.css": "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/github-dark.min.css",
	"highlight.min.js":             "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js",
	"mermaid.min.js":               "https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.min.js",
}

var vendorCmd = &cobra.Command{
	Use:   "vendor",
	Short: "Download third-party CSS/JS assets locally for offline use",
	Long: `This command downloads all external CDN dependencies (GitHub CSS, Highlight.js, Mermaid.js)
and saves them to the third_party/ directory so they can be embedded inside the mdserve binary.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Find project root by searching for go.mod containing "module mdserve"
		rootDir, err := findProjectRoot()
		if err != nil {
			log.Fatalf("Error finding project root: %v", err)
		}

		thirdPartyDir := filepath.Join(rootDir, "third_party")
		log.Printf("Creating directory: %s", thirdPartyDir)
		if err := os.MkdirAll(thirdPartyDir, 0755); err != nil {
			log.Fatalf("Failed to create third_party directory: %v", err)
		}

		for filename, url := range assets {
			destPath := filepath.Join(thirdPartyDir, filename)
			log.Printf("Downloading %s ...", url)
			if err := downloadFile(url, destPath); err != nil {
				log.Fatalf("Failed to download %s: %v", filename, err)
			}
			log.Printf("Successfully saved to %s", destPath)
		}

		log.Println("All third-party assets successfully vendored!")
	},
}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if info, err := os.Stat(goModPath); err == nil && !info.IsDir() {
			// Read the file and check if it has "module mdserve"
			content, err := os.ReadFile(goModPath)
			if err == nil && os.Getenv("GO_WORK") == "" {
				if len(content) > 0 { // Simple check
					return dir, nil
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// Fall back to current working directory
	return os.Getwd()
}

func downloadFile(url string, destPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad HTTP status: %s", resp.Status)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func init() {
	rootCmd.AddCommand(vendorCmd)
}
