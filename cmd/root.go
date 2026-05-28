package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ap",
	Short: "ap (AgilePanel) is a secure, lightweight control panel for WordPress",
	Long:  `A hyper-fast, secure CLI WordPress control panel designed to replace legacy heavy platforms.`,
}

// Execute runs the root CLI command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
