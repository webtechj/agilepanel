package cmd

import (
	"github.com/spf13/cobra"

	"agilepanel/internal/site"
)

var (
	doctorFix bool

	doctorCmd = &cobra.Command{
		Use:   "doctor [domain]",
		Short: "Discover and automatically heal website and server issues (SSL, Caddy, PHP-FPM, DB)",
		Long: `The AgilePanel Doctor is an automated health and auto-remediation engine.
It executes synthetic TLS/SNI probes, HTTP loopback tests, Caddy log analysis,
PHP memory sizing, and database checks to uncover issues (such as Cloudflare 525,
502 Bad Gateway, 500 Fatal Memory Crashes) and resolve them automatically.`,
		Args: cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return site.DoctorWithFix(args[0], doctorFix)
			}
			return site.DoctorAll(doctorFix)
		},
	}
)

func init() {
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "Automatically apply non-destructive fixes for all discovered issues")
	rootCmd.AddCommand(doctorCmd)
}
