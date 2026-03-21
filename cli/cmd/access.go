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

func NewAccessCmd() *cobra.Command {
	access := &cobra.Command{
		Use:   "access",
		Short: "Manage access agreements between publishers",
	}

	access.AddCommand(
		newRequestCmd(),
		newListPendingCmd(),
		newApproveCmd(),
		newDenyCmd(),
		newRevokeCmd(),
		newListAgreementsCmd(),
		newResolveCmd(),
		newGetAgreementCmd(),
	)

	return access
}

// ── sockridge access request ───────────────────────────────────────────────────

func newRequestCmd() *cobra.Command {
	var (
		receiverID string
		message    string
	)

	cmd := &cobra.Command{
		Use:   "request",
		Short: "Request mutual access with another publisher",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := keystore.LoadCredentials()
			if err != nil {
				return err
			}
			if creds.SessionToken == "" {
				return fmt.Errorf("not logged in — run: sockridge auth login")
			}

			c := newClient("")
			resp, err := c.Access.RequestAccess(
				context.Background(),
				connect.NewRequest(&registryv1.RequestAccessRequest{
					RequesterId: creds.PublisherID,
					ReceiverId:  receiverID,
					Message:     message,
				}),
			)
			if err != nil {
				return fmt.Errorf("requesting access: %w", err)
			}

			fmt.Printf("access request sent\n")
			fmt.Printf("agreement id : %s\n", resp.Msg.Agreement.Id)
			fmt.Printf("to publisher : %s\n", receiverID)
			fmt.Printf("status       : %s\n", resp.Msg.Agreement.Status)
			fmt.Printf("message      : %s\n", message)
			return nil
		},
	}

	cmd.Flags().StringVar(&receiverID, "to", "", "Publisher ID to request access from")
	cmd.Flags().StringVar(&message, "message", "", "Why you want to connect")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

// ── sockridge access pending ───────────────────────────────────────────────────

func newListPendingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pending",
		Short: "List incoming access requests waiting for your approval",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := keystore.LoadCredentials()
			if err != nil {
				return err
			}

			c := newClient("")
			stream, err := c.Access.ListPending(
				context.Background(),
				connect.NewRequest(&registryv1.ListPendingRequest{
					PublisherId: creds.PublisherID,
				}),
			)
			if err != nil {
				return fmt.Errorf("listing pending: %w", err)
			}

			count := 0
			for stream.Receive() {
				msg := stream.Msg()
				fmt.Printf("agreement : %s\n", msg.Agreement.Id)
				fmt.Printf("from      : @%s (%s)\n", msg.RequesterHandle, msg.Agreement.RequesterId)
				fmt.Printf("message   : %s\n", msg.Agreement.Message)
				fmt.Printf("---\n")
				count++
			}
			if err := stream.Err(); err != nil && err != io.EOF {
				return fmt.Errorf("stream error: %w", err)
			}
			if count == 0 {
				fmt.Println("no pending requests")
			}
			return nil
		},
	}
	return cmd
}

// ── sockridge access approve ───────────────────────────────────────────────────

func newApproveCmd() *cobra.Command {
	var agreementID string

	cmd := &cobra.Command{
		Use:   "approve",
		Short: "Approve an access request — generates shared key",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := keystore.LoadCredentials()
			if err != nil {
				return err
			}

			c := newClient("")
			resp, err := c.Access.ApproveAccess(
				context.Background(),
				connect.NewRequest(&registryv1.ApproveAccessRequest{
					PublisherId: creds.PublisherID,
					AgreementId: agreementID,
				}),
			)
			if err != nil {
				return fmt.Errorf("approving access: %w", err)
			}

			fmt.Printf("access approved\n")
			fmt.Printf("agreement id : %s\n", resp.Msg.Agreement.Id)
			fmt.Printf("shared key   : %s\n", resp.Msg.SharedKey)
			fmt.Printf("\nShare this key with the requester — they need it to resolve your agent endpoints.\n")
			fmt.Printf("You can use the same key to resolve their agent endpoints.\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&agreementID, "id", "", "Agreement ID to approve")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// ── sockridge access deny ──────────────────────────────────────────────────────

func newDenyCmd() *cobra.Command {
	var agreementID string

	cmd := &cobra.Command{
		Use:   "deny",
		Short: "Deny an access request",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := keystore.LoadCredentials()
			if err != nil {
				return err
			}

			c := newClient("")
			_, err = c.Access.DenyAccess(
				context.Background(),
				connect.NewRequest(&registryv1.DenyAccessRequest{
					PublisherId: creds.PublisherID,
					AgreementId: agreementID,
				}),
			)
			if err != nil {
				return fmt.Errorf("denying access: %w", err)
			}

			fmt.Printf("access request denied\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&agreementID, "id", "", "Agreement ID to deny")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// ── sockridge access revoke ────────────────────────────────────────────────────

func newRevokeCmd() *cobra.Command {
	var agreementID string

	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke an active agreement — both sides lose access",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := keystore.LoadCredentials()
			if err != nil {
				return err
			}

			c := newClient("")
			_, err = c.Access.RevokeAccess(
				context.Background(),
				connect.NewRequest(&registryv1.RevokeAccessRequest{
					PublisherId: creds.PublisherID,
					AgreementId: agreementID,
				}),
			)
			if err != nil {
				return fmt.Errorf("revoking access: %w", err)
			}

			fmt.Printf("agreement revoked — shared key is now invalid\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&agreementID, "id", "", "Agreement ID to revoke")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// ── sockridge access list ──────────────────────────────────────────────────────

func newListAgreementsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all active access agreements",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := keystore.LoadCredentials()
			if err != nil {
				return err
			}

			c := newClient("")
			stream, err := c.Access.ListAgreements(
				context.Background(),
				connect.NewRequest(&registryv1.ListAgreementsRequest{
					PublisherId: creds.PublisherID,
				}),
			)
			if err != nil {
				return fmt.Errorf("listing agreements: %w", err)
			}

			count := 0
			for stream.Receive() {
				msg := stream.Msg()
				fmt.Printf("%-36s  @%-20s  %s\n",
					msg.Agreement.Id,
					msg.OtherHandle,
					msg.Agreement.Status,
				)
				count++
			}
			if err := stream.Err(); err != nil && err != io.EOF {
				return fmt.Errorf("stream error: %w", err)
			}
			if count == 0 {
				fmt.Println("no active agreements")
			}
			return nil
		},
	}
	return cmd
}

// ── sockridge access resolve ───────────────────────────────────────────────────

func newResolveCmd() *cobra.Command {
	var (
		agentID   string
		sharedKey string
	)

	cmd := &cobra.Command{
		Use:   "resolve",
		Short: "Resolve an agent's endpoint URL using a shared key",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient("")
			resp, err := c.Access.ResolveEndpoint(
				context.Background(),
				connect.NewRequest(&registryv1.ResolveEndpointRequest{
					AgentId:   agentID,
					SharedKey: sharedKey,
				}),
			)
			if err != nil {
				return fmt.Errorf("resolving endpoint: %w", err)
			}

			if resp.Msg.Agent != nil {
				printAgentDetail(resp.Msg.Agent)
			} else {
				fmt.Printf("url       : %s\n", resp.Msg.Url)
				fmt.Printf("transport : %s\n", resp.Msg.Transport)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&agentID, "agent", "", "Agent ID to resolve")
	cmd.Flags().StringVar(&sharedKey, "key", "", "Shared key from access agreement")
	_ = cmd.MarkFlagRequired("agent")
	_ = cmd.MarkFlagRequired("key")
	return cmd
}

// ── sockridge access get ───────────────────────────────────────────────────────

func newGetAgreementCmd() *cobra.Command {
	var agreementID string

	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get agreement details including shared key",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := keystore.LoadCredentials()
			if err != nil {
				return err
			}

			c := newClient("")
			resp, err := c.Access.GetAgreement(
				context.Background(),
				connect.NewRequest(&registryv1.GetAgreementRequest{
					PublisherId: creds.PublisherID,
					AgreementId: agreementID,
				}),
			)
			if err != nil {
				return fmt.Errorf("getting agreement: %w", err)
			}

			a := resp.Msg.Agreement
			fmt.Printf("agreement id : %s\n", a.Id)
			fmt.Printf("status       : %s\n", a.Status)
			fmt.Printf("requester    : %s\n", a.RequesterId)
			fmt.Printf("receiver     : %s\n", a.ReceiverId)
			if a.SharedKey != "" {
				fmt.Printf("shared key   : %s\n", a.SharedKey)
			} else {
				fmt.Printf("shared key   : (pending approval)\n")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&agreementID, "id", "", "Agreement ID")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}