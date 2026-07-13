package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	portFlag int
	dirFlag  string
	allFlag  bool
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "mdserve",
	Short: "mdserve is a local markdown and file viewer server inspired by GitHub's code view.",
	Long: `mdserve is a local markdown and file viewer server inspired by GitHub's code view.
It serves markdown files in the specified directory, rendering them in a beautiful, responsive,
github-like interface with sidebar navigation and hot-reload.

By default, running 'mdserve' starts the web server on port 8080 and serves files from the current directory.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServer(portFlag, dirFlag, allFlag)
	},
}

// startCmd represents the start command to launch the server explicitly.
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the markdown web server",
	Long:  `Start the markdown web server with specified parameters (port, directory, all files).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServer(portFlag, dirFlag, allFlag)
	},
}

// installCmd represents the install command to copy the binary to a user path.
var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install mdserve binary to user's binary path",
	Long: `Install mdserve binary to the user's binary path.

The installation directory is determined using the following algorithm:
1. Reads ~/.mdserve/config.jsonc. If 'install_dir' is specified and not empty, it is used (supporting home directory tilde extension).
2. If not configured, checks if ~/.local/bin exists. If it does, ~/.local/bin is used.
3. If not, checks if ~/bin exists. If it does, ~/bin is used.
4. If neither directory exists, the installer defaults to ~/.local/bin and creates it.

If the chosen installation directory is not in your current PATH, a warning will be displayed.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInstaller()
	},
}

func init() {
	// Flags for root command and start command
	rootCmd.PersistentFlags().IntVarP(&portFlag, "port", "p", 8080, "Port to run server on")
	rootCmd.PersistentFlags().StringVarP(&dirFlag, "dir", "d", ".", "Directory of Markdown files to serve")
	rootCmd.PersistentFlags().BoolVarP(&allFlag, "all", "a", false, "Show all files, not just .md files")

	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(installCmd)
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
