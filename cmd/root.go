package cmd

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const Version = "0.8.0"

var rootCmd = &cobra.Command{
	Use:     "ap",
	Version: Version,
	Short:   "ap (AgilePanel) is a secure, lightweight control panel for WordPress",
	Long:    `A hyper-fast, secure CLI WordPress control panel designed to replace legacy heavy platforms.`,
}

// Execute runs the root CLI command.
func Execute() {
	// Silence default Cobra error printing so we can print our own premium styled error
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true

	// Check for root privileges on Linux to guide newbies
	if runtime.GOOS == "linux" && os.Getuid() != 0 {
		fmt.Println()
		fmt.Printf("  \033[91m✘  Permission Denied:\033[0m \033[1;97mAgilePanel must be run with root privileges.\033[0m\n")
		fmt.Println("  \033[90m────────────────────────────────────────────────────────────────────────\033[0m")
		fmt.Println("  \033[34m›\033[0m  \033[93m💡 Ubuntu Server Tip:\033[0m Most commands (like managing services, editing configuration")
		fmt.Println("     files, and configuring firewall/SSH rules) require root-level permissions.")
		fmt.Printf("  \033[34m›\033[0m  \033[2mWhat to do next:\033[0m Prefix your command with 'sudo', e.g., \033[1;32msudo ap %s\033[0m\n", strings.Join(os.Args[1:], " "))
		fmt.Println("  \033[90m────────────────────────────────────────────────────────────────────────\033[0m")
		fmt.Println()
		os.Exit(1)
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Println()
		fmt.Printf("  \033[91m✘  Operation Failed:\033[0m \033[1;97m%s\033[0m\n", err.Error())
		fmt.Println("  \033[90m────────────────────────────────────────────────────────────────────────\033[0m")
		fmt.Println("  \033[34m›\033[0m  \033[2mWhat went wrong:\033[0m The action was stopped due to the error listed above.")
		
		// If command typo is detected, suggest the closest matching command nomenclature
		if strings.Contains(err.Error(), "unknown command") {
			quoteParts := strings.Split(err.Error(), "\"")
			if len(quoteParts) >= 2 {
				unknownCmd := quoteParts[1]
				parent := rootCmd
				for _, arg := range os.Args[1:] {
					sub, _, _ := parent.Find([]string{arg})
					if sub != nil && sub != parent {
						parent = sub
					}
				}
				suggestions := parent.SuggestionsFor(unknownCmd)
				if len(suggestions) > 0 {
					fmt.Printf("  \033[34m›\033[0m  \033[93mDid you mean:\033[0m \033[1;32m%s %s\033[0m?\n", parent.CommandPath(), strings.Join(suggestions, ", "))
				}
			}
		}

		fmt.Println("  \033[34m›\033[0m  \033[2mWhat to do next:\033[0m Double-check the command spelling, verify the domain name is")
		fmt.Println("     correctly formatted and exists, and ensure you are running as root/sudo.")
		fmt.Println("  \033[90m────────────────────────────────────────────────────────────────────────\033[0m")
		fmt.Println()
		os.Exit(1)
	}
}
