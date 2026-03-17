package main

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/utsav-develops/SocialAgents/cli/cmd"
)

func main() {
	root := &cobra.Command{
		Use:   "agentctl",
		Short: "CLI for the agent registry",
		Long:  "agentctl — publish, discover, and manage agents in the registry.",
	}

	root.AddCommand(
		cmd.NewAuthCmd(),
		cmd.NewPublishCmd(),
		cmd.NewSearchCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
