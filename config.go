package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Config holds user-configurable options for the mdserve application.
type Config struct {
	InstallDir string `json:"install_dir"`
}

// loadConfig loads and parses config.jsonc from the ~/.mdserve directory.
// If the configuration file does not exist, an empty Config is returned.
func loadConfig() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("error resolving user home directory: %w", err)
	}

	configDir := filepath.Join(home, ".mdserve")
	configPath := filepath.Join(configDir, "config.jsonc")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &Config{}, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	cleanData := stripComments(data)
	var config Config
	if err := json.Unmarshal(cleanData, &config); err != nil {
		return nil, fmt.Errorf("error parsing json config: %w", err)
	}

	return &config, nil
}

// stripComments removes single-line (//) and multi-line (/* */) comments from raw JSONC bytes.
func stripComments(data []byte) []byte {
	var result []byte
	inSingleLine := false
	inMultiLine := false
	i := 0
	n := len(data)
	for i < n {
		if inSingleLine {
			if data[i] == '\n' {
				inSingleLine = false
				result = append(result, '\n')
			}
			i++
			continue
		}
		if inMultiLine {
			if i+1 < n && data[i] == '*' && data[i+1] == '/' {
				inMultiLine = false
				i += 2
			} else {
				i++
			}
			continue
		}
		if i+1 < n && data[i] == '/' && data[i+1] == '/' {
			inSingleLine = true
			i += 2
		} else if i+1 < n && data[i] == '/' && data[i+1] == '*' {
			inMultiLine = true
			i += 2
		} else {
			result = append(result, data[i])
			i++
		}
	}
	return result
}

// expandHomePath expands the leading ~ in a path to the user's home directory.
func expandHomePath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[1:])
	}
	return filepath.Clean(path), nil
}

// determineInstallDir evaluates the default installation directory algorithm.
func determineInstallDir(cfg *Config) (string, error) {
	if cfg.InstallDir != "" {
		expanded, err := expandHomePath(cfg.InstallDir)
		if err != nil {
			return "", fmt.Errorf("invalid install_dir path: %w", err)
		}
		return expanded, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("error resolving user home: %w", err)
	}

	localBin := filepath.Join(home, ".local", "bin")
	if _, err := os.Stat(localBin); err == nil {
		return localBin, nil
	}

	userBin := filepath.Join(home, "bin")
	if _, err := os.Stat(userBin); err == nil {
		return userBin, nil
	}

	return localBin, nil
}

// isDirInPath returns true if the specified directory is part of the user's PATH environment.
func isDirInPath(dir string) bool {
	pathEnv := os.Getenv("PATH")
	paths := filepath.SplitList(pathEnv)
	cleanDir := filepath.Clean(dir)
	for _, p := range paths {
		if filepath.Clean(p) == cleanDir {
			return true
		}
	}
	return false
}

// runInstaller handles compiling/copying the running binary to the determined directory.
func runInstaller() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	installDir, err := determineInstallDir(cfg)
	if err != nil {
		return err
	}

	log.Printf("Installing mdserve to: %s", installDir)

	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("error creating install directory: %w", err)
	}

	src, err := os.Executable()
	if err != nil {
		return fmt.Errorf("error identifying currently running executable: %w", err)
	}

	dst := filepath.Join(installDir, "mdserve")
	if err := copyFile(src, dst); err != nil {
		return fmt.Errorf("error copying executable: %w", err)
	}

	if err := os.Chmod(dst, 0755); err != nil {
		return fmt.Errorf("error setting executable permissions: %w", err)
	}

	log.Printf("Successfully installed mdserve to %s", dst)

	// Write template config if it doesn't exist
	home, err := os.UserHomeDir()
	if err == nil {
		configDir := filepath.Join(home, ".mdserve")
		configPath := filepath.Join(configDir, "config.jsonc")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			_ = os.MkdirAll(configDir, 0755)
			templateData := `{// Configuration options for mdserve
  // The directory where 'mdserve install' will copy the compiled binary.
  // If not specified, the installer uses ~/.local/bin or ~/bin depending on existence.
  "install_dir": ""
}`
			_ = os.WriteFile(configPath, []byte(templateData), 0644)
			log.Printf("Created default config template at: %s", configPath)
		}
	}

	// Verify PATH
	if !isDirInPath(installDir) {
		fmt.Printf("\n[WARNING] The installation directory %s is not in your PATH environment variable.\n", installDir)
		fmt.Println("To run 'mdserve' from anywhere, add this to your shell configuration (e.g. ~/.bashrc or ~/.zshrc):")
		fmt.Printf("  export PATH=\"$PATH:%s\"\n\n", installDir)
	}

	return nil
}

// copyFile copies the contents of src file to dst file.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
