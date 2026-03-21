package main

import (
	"os"

	"github.com/Sockridge/sockridge/cli/cmd"
	"github.com/Sockridge/sockridge/cli/internal/keystore"
	"github.com/spf13/cobra"
)

func main() {
	var configDir string
 
	root := &cobra.Command{
		Use:   "agentctl",
		Short: "CLI for the agent registry",
		Long:  "agentctl — publish, discover, and manage agents in the registry.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if configDir != "" {
				keystore.SetConfigDir(configDir)
			}
		},
	}
 
	root.PersistentFlags().StringVar(&configDir, "config", "", "Config directory (default ~/.agentctl)")
 
	root.AddCommand(
		cmd.NewAuthCmd(),
		cmd.NewPublishCmd(),
		cmd.NewSearchCmd(),
		cmd.NewAccessCmd(),
		cmd.NewAuditCmd(),
	)
 
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
 