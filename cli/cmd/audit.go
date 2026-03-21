package cmd

import (
	"context"
	"fmt"
	"io"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	"github.com/Sockridge/sockridge/cli/internal/keystore"
	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
)

func NewAuditCmd() *cobra.Command {
	audit := &cobra.Command{
		Use:   "audit",
		Short: "View audit log for your account",
	}
	audit.AddCommand(newAuditListCmd())
	return audit
}

func newAuditListCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent audit events",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := keystore.LoadCredentials()
			if err != nil {
				return fmt.Errorf("not logged in — run: sockridge auth login")
			}
			if creds == nil || creds.PublisherID == "" {
				return fmt.Errorf("not logged in — run: sockridge auth login")
			}
			if creds.SessionToken == "" {
				return fmt.Errorf("session expired — run: sockridge auth login")
			}

			c := newClient("")
			stream, err := c.Audit.ListEvents(
				context.Background(),
				connect.NewRequest(&registryv1.ListAuditEventsRequest{
					PublisherId: creds.PublisherID,
					Limit:       uint32(limit),
				}),
			)
			if err != nil {
				return fmt.Errorf("listing audit events: %w", err)
			}

			count := 0
			for stream.Receive() {
				e := stream.Msg().Event
				ts := ""
				if e.OccurredAt != nil {
					ts = e.OccurredAt.AsTime().Format("2006-01-02 15:04:05")
				}
				agentInfo := ""
				if e.AgentId != "" {
					agentInfo = fmt.Sprintf(" agent=%s", e.AgentId[:8])
				}
				ipInfo := ""
				if e.Ip != "" {
					ipInfo = fmt.Sprintf(" ip=%s", e.Ip)
				}
				fmt.Printf("%s  %-20s%s%s  %s\n", ts, e.Action, agentInfo, ipInfo, e.Detail)
				count++
			}
			if err := stream.Err(); err != nil && err != io.EOF {
				return fmt.Errorf("stream error: %w", err)
			}
			if count == 0 {
				fmt.Println("no audit events found")
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum events to show")
	return cmd
}