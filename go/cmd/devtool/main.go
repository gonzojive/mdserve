package main

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "devtool",
	Short: "devtool is a CLI utility for mdserve development tasks",
	Long:  `A developer tool for mdserve to capture screenshots and perform other tasks.`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
