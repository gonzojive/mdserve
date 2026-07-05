package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var screenshotCmd = &cobra.Command{
	Use:   "screenshot",
	Short: "Start mdserve and capture a headless browser screenshot of the rendered page",
	Long: `This command starts the mdserve Markdown server using the provided arguments,
waits for the server to be ready, launches headless Google Chrome to capture a
screenshot of the page, saves it to screenshot.png, and stops the server.

Example:
  devtool screenshot -- -port 18080 -dir .
`,
	Run: func(cmd *cobra.Command, args []string) {
		// Extract raw arguments after "--"
		var mdserveArgs []string
		for i, arg := range os.Args {
			if arg == "--" {
				mdserveArgs = os.Args[i+1:]
				break
			}
		}

		// Check if mdserve binary exists in current directory, compile if not
		mdservePath := "./mdserve"
		if _, err := os.Stat(mdservePath); os.IsNotExist(err) {
			log.Println("mdserve binary not found in current directory. Compiling main.go...")
			buildCmd := exec.Command("go", "build", "-o", "mdserve", "main.go")
			buildCmd.Stdout = os.Stdout
			buildCmd.Stderr = os.Stderr
			if err := buildCmd.Run(); err != nil {
				log.Fatalf("Failed to compile mdserve: %v", err)
			}
			log.Println("Compilation completed successfully.")
		}

		// Determine the port
		port := getPort(mdserveArgs)
		log.Printf("Starting mdserve on port %s...", port)

		// Start mdserve in the background
		serverCmd := exec.Command(mdservePath, mdserveArgs...)
		serverCmd.Stdout = os.Stdout
		serverCmd.Stderr = os.Stderr
		if err := serverCmd.Start(); err != nil {
			log.Fatalf("Failed to start mdserve: %v", err)
		}

		// Ensure the server process is killed when the command exits
		defer func() {
			if serverCmd.Process != nil {
				log.Println("Stopping mdserve server...")
				serverCmd.Process.Kill()
				serverCmd.Wait()
			}
		}()

		// Poll server until it is ready
		url := fmt.Sprintf("http://localhost:%s/", port)
		log.Printf("Waiting for server to be ready at %s...", url)
		ready := false
		for i := 0; i < 50; i++ {
			resp, err := http.Get(url)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					ready = true
					break
				}
			}
			time.Sleep(100 * time.Millisecond)
		}

		if !ready {
			log.Fatalf("Error: Server failed to start and respond with HTTP 200 within 5 seconds")
		}
		log.Println("Server is ready. Capturing screenshot...")

		// Look up Google Chrome / Chromium
		chromePath, err := exec.LookPath("google-chrome")
		if err != nil {
			chromePath, err = exec.LookPath("chromium-browser")
			if err != nil {
				chromePath = "google-chrome" // Fallback
			}
		}

		screenshotFile, _ := filepath.Abs("screenshot.png")
		chromeCmd := exec.Command(chromePath,
			"--headless",
			"--disable-gpu",
			fmt.Sprintf("--screenshot=%s", screenshotFile),
			"--window-size=1280,800",
			url,
		)
		chromeCmd.Stdout = os.Stdout
		chromeCmd.Stderr = os.Stderr

		if err := chromeCmd.Run(); err != nil {
			log.Fatalf("Failed to run Google Chrome screenshot: %v", err)
		}

		log.Printf("Screenshot successfully saved to %s", screenshotFile)
	},
}

func getPort(args []string) string {
	for i, arg := range args {
		if strings.HasPrefix(arg, "-port=") {
			return strings.TrimPrefix(arg, "-port=")
		}
		if strings.HasPrefix(arg, "--port=") {
			return strings.TrimPrefix(arg, "--port=")
		}
		if (arg == "-port" || arg == "--port") && i+1 < len(args) {
			return args[i+1]
		}
	}
	return "8080" // Default port
}

func init() {
	rootCmd.AddCommand(screenshotCmd)
}
