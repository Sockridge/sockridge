package cmd

import (
	"context"
	"fmt"
	"os"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

registryv1 "github.com/utsav-develops/SocialAgents/server/gen/go/agentregistry/v1"
"github.com/utsav-develops/SocialAgents/cli/internal/client"
"github.com/utsav-develops/SocialAgents/cli/internal/keystore"
)

func NewAuthCmd() *cobra.Command {
	auth := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication and keypairs",
	}

	auth.AddCommand(
		newKeygenCmd(),
		newRegisterCmd(),
		newLoginCmd(),
		newWhoamiCmd(),
	)

	return auth
}

// ── agentctl auth keygen ──────────────────────────────────────────────────────

func newKeygenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "keygen",
		Short: "Generate a new Ed25519 keypair",
		Long:  "Generates an Ed25519 keypair and saves the private key to ~/.agentctl/ed25519.key (chmod 600).",
		RunE: func(cmd *cobra.Command, args []string) error {
			kp, err := keystore.Generate()
			if err != nil {
				return err
			}

			fmt.Println("keypair generated")
			fmt.Printf("public key : %s\n", kp.PublicKey)
			fmt.Printf("private key: ~/.agentctl/ed25519.key (keep this secret)\n")
			return nil
		},
	}
}

// ── agentctl auth register ────────────────────────────────────────────────────

func newRegisterCmd() *cobra.Command {
	var (
		handle    string
		serverURL string
	)

	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register a new publisher account",
		Long:  "Registers your public key with the registry under a chosen handle.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if handle == "" {
				return fmt.Errorf("--handle is required")
			}

			kp, err := keystore.Load()
			if err != nil {
				return err
			}

			c := client.New(serverURL, "")
			resp, err := c.Registry.RegisterPublisher(
				context.Background(),
				connect.NewRequest(&registryv1.RegisterPublisherRequest{
					Handle:    handle,
					PublicKey: kp.PublicKey,
				}),
			)
			if err != nil {
				return fmt.Errorf("registering publisher: %w", err)
			}

			creds := &keystore.Credentials{
				PublisherID: resp.Msg.PublisherId,
				Handle:      handle,
				ServerURL:   serverURL,
			}

			if err := keystore.SaveCredentials(creds); err != nil {
				return err
			}

			fmt.Printf("registered as @%s\n", handle)
			fmt.Printf("publisher id: %s\n", resp.Msg.PublisherId)
			fmt.Printf("credentials saved to ~/.agentctl/credentials.json\n")
			return nil
		},
	}

	cmd.Flags().StringVar(&handle, "handle", "", "Your publisher handle (e.g. utsav)")
	cmd.Flags().StringVar(&serverURL, "server", client.DefaultServerURL, "Registry server URL")

	return cmd
}

// ── agentctl auth login ───────────────────────────────────────────────────────
// Performs the full challenge-response flow and prints the session token.
// The token is stored in ~/.agentctl/credentials.json for use by other commands.

func newLoginCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate and get a session token",
		Long:  "Performs Ed25519 challenge-response auth and saves the session token locally.",
		RunE: func(cmd *cobra.Command, args []string) error {
			kp, err := keystore.Load()
			if err != nil {
				return err
			}

			creds, err := keystore.LoadCredentials()
			if err != nil {
				return err
			}

			if serverURL != client.DefaultServerURL {
				creds.ServerURL = serverURL
			}

			c := client.New(creds.ServerURL, "")

			// step 1: get challenge nonce
			chalResp, err := c.Registry.AuthChallenge(
				context.Background(),
				connect.NewRequest(&registryv1.AuthChallengeRequest{
					PublisherId: creds.PublisherID,
				}),
			)
			if err != nil {
				return fmt.Errorf("requesting challenge: %w", err)
			}

			// step 2: sign the nonce
			sig := kp.Sign([]byte(chalResp.Msg.Nonce))

			// step 3: verify and get session token
			verifyResp, err := c.Registry.AuthVerify(
				context.Background(),
				connect.NewRequest(&registryv1.AuthVerifyRequest{
					PublisherId: creds.PublisherID,
					Nonce:       chalResp.Msg.Nonce,
					Signature:   sig,
				}),
			)
			if err != nil {
				return fmt.Errorf("verifying signature: %w", err)
			}

			// save token to credentials
			creds.SessionToken = verifyResp.Msg.SessionToken
			if err := keystore.SaveCredentials(creds); err != nil {
				return err
			}

			fmt.Printf("logged in as @%s\n", creds.Handle)
			fmt.Printf("session token saved (expires: %s)\n",
				verifyResp.Msg.ExpiresAt.AsTime().Format("15:04:05 MST"))
			return nil
		},
	}

	cmd.Flags().StringVar(&serverURL, "server", client.DefaultServerURL, "Registry server URL")
	return cmd
}

// ── agentctl auth whoami ──────────────────────────────────────────────────────

func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show current authenticated identity",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := keystore.LoadCredentials()
			if err != nil {
				return err
			}

			fmt.Printf("handle      : @%s\n", creds.Handle)
			fmt.Printf("publisher id: %s\n", creds.PublisherID)
			fmt.Printf("server      : %s\n", creds.ServerURL)

			if creds.SessionToken == "" {
				fmt.Fprintln(os.Stderr, "not logged in — run: agentctl auth login")
			}

			return nil
		},
	}
}
