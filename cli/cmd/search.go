package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	"github.com/Sockridge/sockridge/cli/internal/client"
	"github.com/Sockridge/sockridge/cli/internal/keystore"
	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
)

func NewSearchCmd() *cobra.Command {
	search := &cobra.Command{
		Use:   "search",
		Short: "Discover agents in the registry",
	}

	search.AddCommand(
		newListCmd(),
		newGetCmd(),
		newSemanticCmd(),
		newWatchCmd(),
		newMyAgentCmd(),
	)

	return search
}

// ── agentctl search list ──────────────────────────────────────────────────────

func newListCmd() *cobra.Command {
	var (
		tags      []string
		limit     int
		serverURL string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List active agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(serverURL)

			stream, err := c.Discovery.ListAgents(
				context.Background(),
				connect.NewRequest(&registryv1.ListAgentsRequest{
					Tags:  tags,
					Limit: uint32(limit),
				}),
			)
			if err != nil {
				return fmt.Errorf("listing agents: %w", err)
			}

			count := 0
			for stream.Receive() {
				printAgentRow(stream.Msg().Agent)
				count++
			}
			if err := stream.Err(); err != nil && err != io.EOF {
				return fmt.Errorf("stream error: %w", err)
			}
			if count == 0 {
				fmt.Println("no agents found")
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Filter by tag (repeatable: --tag fhir --tag labs)")
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum results")
	cmd.Flags().StringVar(&serverURL, "server", client.DefaultServerURL, "Registry server URL")
	return cmd
}

// ── agentctl search get ───────────────────────────────────────────────────────

func newGetCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "get <agent-id>",
		Short: "Get a single agent by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(serverURL)

			resp, err := c.Discovery.GetAgent(
				context.Background(),
				connect.NewRequest(&registryv1.DiscoveryServiceGetAgentRequest{
					AgentId: args[0],
				}),
			)
			if err != nil {
				return fmt.Errorf("getting agent: %w", err)
			}

			printAgentDetail(resp.Msg.Agent)
			return nil
		},
	}

	cmd.Flags().StringVar(&serverURL, "server", client.DefaultServerURL, "Registry server URL")
	return cmd
}

// ── agentctl search semantic ──────────────────────────────────────────────────

func newSemanticCmd() *cobra.Command {
	var (
		topK      int
		minScore  float32
		serverURL string
	)

	cmd := &cobra.Command{
		Use:   "semantic <query>",
		Short: "Find agents by natural language description",
		Long:  `Uses vector similarity to find agents matching a natural language query.`,
		Args:  cobra.MinimumNArgs(1),
		Example: `  agentctl search semantic "analyze lab trends from FHIR"
  agentctl search semantic "detect drug interactions" --top-k 5
  agentctl search semantic "calendar management" --min-score 0.5`,
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			c := newClient(serverURL)

			stream, err := c.Discovery.SemanticSearch(
				context.Background(),
				connect.NewRequest(&registryv1.SemanticSearchRequest{
					Query:    query,
					TopK:     uint32(topK),
					MinScore: minScore,
				}),
			)
			if err != nil {
				return fmt.Errorf("semantic search: %w", err)
			}

			count := 0
			for stream.Receive() {
				msg := stream.Msg()
				fmt.Printf("%.2f  %-36s  %s\n", msg.Score, msg.Agent.Id, msg.Agent.Name)
				count++
			}
			if err := stream.Err(); err != nil && err != io.EOF {
				return fmt.Errorf("stream error: %w", err)
			}
			if count == 0 {
				fmt.Println("no matching agents found")
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&topK, "top-k", 10, "Maximum results")
	cmd.Flags().Float32Var(&minScore, "min-score", 0.3, "Minimum similarity score (0.0-1.0)")
	cmd.Flags().StringVar(&serverURL, "server", client.DefaultServerURL, "Registry server URL")
	return cmd
}

// ── agentctl search watch ─────────────────────────────────────────────────────

func newWatchCmd() *cobra.Command {
	var (
		tags      []string
		serverURL string
	)

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Stream live agent publish/update/deprecate events",
		Long:  "Opens a long-lived stream. Press Ctrl+C to stop.",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(serverURL)

			fmt.Printf("watching for agents")
			if len(tags) > 0 {
				fmt.Printf(" (tags: %s)", strings.Join(tags, ", "))
			}
			fmt.Println(" — press Ctrl+C to stop")

			stream, err := c.Discovery.Watch(
				context.Background(),
				connect.NewRequest(&registryv1.WatchRequest{Tags: tags}),
			)
			if err != nil {
				return fmt.Errorf("starting watch: %w", err)
			}

			for stream.Receive() {
				msg := stream.Msg()
				eventLabel := strings.ToLower(strings.TrimPrefix(msg.EventType.String(), "EVENT_TYPE_"))
				fmt.Printf("[%s] %s — %s v%s\n", eventLabel, msg.Agent.Id, msg.Agent.Name, msg.Agent.Version)
			}

			if err := stream.Err(); err != nil && err != io.EOF {
				return fmt.Errorf("watch stream error: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&tags, "tag", nil, "Watch agents with this tag (repeatable)")
	cmd.Flags().StringVar(&serverURL, "server", client.DefaultServerURL, "Registry server URL")
	return cmd
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newClient(serverURL string) *client.Client {
	if serverURL == "" {
		serverURL = client.DefaultServerURL
	}
	creds, _ := keystore.LoadCredentials()
	if creds != nil && creds.ServerURL != "" && serverURL == client.DefaultServerURL {
		serverURL = creds.ServerURL
	}
	token := ""
	if creds != nil {
		token = creds.SessionToken
	}
	return client.New(serverURL, token)
}

func printAgentRow(a *registryv1.AgentCard) {
	skills := make([]string, len(a.Skills))
	for i, s := range a.Skills {
		skills[i] = s.Id
	}
	fmt.Printf("%-36s  %-24s  v%-8s  %s\n",
		a.Id, a.Name, a.Version, strings.Join(skills, ", "))
}

func printAgentDetail(a *registryv1.AgentCard) {
	fmt.Printf("id          : %s\n", a.Id)
	fmt.Printf("name        : %s\n", a.Name)
	fmt.Printf("description : %s\n", a.Description)
	fmt.Printf("version     : %s\n", a.Version)
	fmt.Printf("url         : %s\n", a.Url)
	fmt.Printf("publisher   : %s\n", a.PublisherId)
	fmt.Printf("status      : %s\n", a.Status)
	fmt.Printf("skills      :\n")
	for _, s := range a.Skills {
		fmt.Printf("  - %s (%s) [%s]\n", s.Name, s.Id, strings.Join(s.Tags, ", "))
	}
}

// ── agentctl search mine ──────────────────────────────────────────────────────
// Gets your own agent with full details including URL (requires auth)

func newMyAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mine <agent-id>",
		Short: "Get your own agent with full details including URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := keystore.LoadCredentials()
			if err != nil {
				return err
			}
			if creds.SessionToken == "" {
				return fmt.Errorf("not logged in — run: agentctl auth login")
			}

			c := newClient("")
			resp, err := c.Registry.GetAgent(
				context.Background(),
				connect.NewRequest(&registryv1.RegistryServiceGetAgentRequest{
					AgentId: args[0],
				}),
			)
			if err != nil {
				return fmt.Errorf("getting agent: %w", err)
			}

			printAgentDetail(resp.Msg.Agent)
			return nil
		},
	}
	return cmd
}