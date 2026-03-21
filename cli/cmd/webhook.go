package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	"github.com/Sockridge/sockridge/cli/internal/keystore"
	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
)

func NewWebhookCmd() *cobra.Command {
	wh := &cobra.Command{
		Use:   "webhook",
		Short: "Manage webhooks for push notifications",
	}
	wh.AddCommand(
		newWebhookRegisterCmd(),
		newWebhookListCmd(),
		newWebhookDeleteCmd(),
		newWebhookTestCmd(),
	)
	return wh
}

func newWebhookRegisterCmd() *cobra.Command {
	var (
		url    string
		events []string
	)
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register a webhook URL",
		Example: `  sockridge webhook register --url https://myserver.com/hooks --event access_request --event agent_active
  
  Available events:
    access_request   — someone requests access to your agents
    access_approved  — an access request you sent was approved
    access_denied    — an access request you sent was denied
    access_revoked   — an active agreement was revoked
    agent_active     — your agent passed gatekeeper and went live
    agent_inactive   — your agent failed health checks and went inactive
    agent_published  — confirmation your agent was published
    agent_rejected   — your agent failed gatekeeper`,
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := keystore.LoadCredentials()
			if err != nil || creds == nil {
				return fmt.Errorf("not logged in — run: sockridge auth login")
			}
			c := newClient("")
			resp, err := c.Webhook.RegisterWebhook(
				context.Background(),
				connect.NewRequest(&registryv1.RegisterWebhookRequest{
					PublisherId: creds.PublisherID,
					Url:         url,
					Events:      events,
				}),
			)
			if err != nil {
				return fmt.Errorf("registering webhook: %w", err)
			}
			fmt.Printf("webhook registered\n")
			fmt.Printf("id     : %s\n", resp.Msg.Webhook.Id)
			fmt.Printf("url    : %s\n", resp.Msg.Webhook.Url)
			fmt.Printf("events : %s\n", strings.Join(resp.Msg.Webhook.Events, ", "))
			fmt.Printf("secret : %s\n", resp.Msg.Secret)
			fmt.Printf("\nSave the secret — it won't be shown again.\n")
			fmt.Printf("Use it to verify webhook signatures: X-Sockridge-Signature: sha256=<hmac>\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&url, "url", "", "HTTPS URL to receive webhook events")
	cmd.Flags().StringArrayVar(&events, "event", nil, "Event to subscribe to (repeatable)")
	_ = cmd.MarkFlagRequired("url")
	_ = cmd.MarkFlagRequired("event")
	return cmd
}

func newWebhookListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered webhooks",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := keystore.LoadCredentials()
			if err != nil || creds == nil {
				return fmt.Errorf("not logged in — run: sockridge auth login")
			}
			c := newClient("")
			stream, err := c.Webhook.ListWebhooks(
				context.Background(),
				connect.NewRequest(&registryv1.ListWebhooksRequest{
					PublisherId: creds.PublisherID,
				}),
			)
			if err != nil {
				return fmt.Errorf("listing webhooks: %w", err)
			}
			count := 0
			for stream.Receive() {
				wh := stream.Msg().Webhook
				status := "active"
				if !wh.Active {
					status = "inactive"
				}
				fmt.Printf("%-36s  %-8s  %-40s  %s\n",
					wh.Id, status, wh.Url, strings.Join(wh.Events, ", "))
				count++
			}
			if err := stream.Err(); err != nil && err != io.EOF {
				return fmt.Errorf("stream error: %w", err)
			}
			if count == 0 {
				fmt.Println("no webhooks registered")
			}
			return nil
		},
	}
	return cmd
}

func newWebhookDeleteCmd() *cobra.Command {
	var webhookID string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a webhook",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := keystore.LoadCredentials()
			if err != nil || creds == nil {
				return fmt.Errorf("not logged in — run: sockridge auth login")
			}
			c := newClient("")
			_, err = c.Webhook.DeleteWebhook(
				context.Background(),
				connect.NewRequest(&registryv1.DeleteWebhookRequest{
					PublisherId: creds.PublisherID,
					WebhookId:   webhookID,
				}),
			)
			if err != nil {
				return fmt.Errorf("deleting webhook: %w", err)
			}
			fmt.Println("webhook deleted")
			return nil
		},
	}
	cmd.Flags().StringVar(&webhookID, "id", "", "Webhook ID to delete")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func newWebhookTestCmd() *cobra.Command {
	var webhookID string
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Send a test event to a webhook",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := keystore.LoadCredentials()
			if err != nil || creds == nil {
				return fmt.Errorf("not logged in — run: sockridge auth login")
			}
			c := newClient("")
			resp, err := c.Webhook.TestWebhook(
				context.Background(),
				connect.NewRequest(&registryv1.TestWebhookRequest{
					PublisherId: creds.PublisherID,
					WebhookId:   webhookID,
				}),
			)
			if err != nil {
				return fmt.Errorf("testing webhook: %w", err)
			}
			if resp.Msg.Success {
				fmt.Printf("✓ webhook responded with %d\n", resp.Msg.StatusCode)
			} else {
				fmt.Printf("✗ webhook failed: %s (status %d)\n", resp.Msg.Error, resp.Msg.StatusCode)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&webhookID, "id", "", "Webhook ID to test")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}