package cmd

import (
	"context"
	"fmt"
	"os"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/Sockridge/sockridge/cli/internal/client"
	"github.com/Sockridge/sockridge/cli/internal/keystore"
	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
)

func NewPublishCmd() *cobra.Command {
	var (
		agentFile string
		serverURL string
	)

	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish or update an agent in the registry",
		Long: `Publish reads an AgentCard from a JSON file, signs the proto bytes
with your Ed25519 private key, and sends it to the registry.

Example agent file (agent.json):
  {
    "name": "My FHIR Agent",
    "description": "Analyzes lab trends",
    "version": "1.0.0",
    "protocol_version": "0.3.0",
    "skills": [
      { "id": "lab.analyze", "name": "Lab Analyzer", "tags": ["fhir", "labs"] }
    ],
    "capabilities": { "streaming": true }
  }`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if agentFile == "" {
				return fmt.Errorf("--file is required")
			}

			// load credentials + keypair
			creds, err := keystore.LoadCredentials()
			if err != nil {
				return err
			}
			if creds.SessionToken == "" {
				return fmt.Errorf("not logged in — run: sockridge auth login")
			}

			kp, err := keystore.Load()
			if err != nil {
				return err
			}

			// read and parse the agent JSON file
			data, err := os.ReadFile(agentFile)
			if err != nil {
				return fmt.Errorf("reading agent file: %w", err)
			}

			var agent registryv1.AgentCard
			if err := protojson.Unmarshal(data, &agent); err != nil {
				return fmt.Errorf("parsing agent file: %w", err)
			}

			// serialize to proto bytes — this is what gets signed
			payload, err := proto.Marshal(&agent)
			if err != nil {
				return fmt.Errorf("serializing agent: %w", err)
			}

			// sign the proto bytes with Ed25519 private key
			sig := kp.Sign(payload)

			if serverURL != client.DefaultServerURL {
				creds.ServerURL = serverURL
			}

			c := client.New(creds.ServerURL, creds.SessionToken)

			resp, err := c.Registry.PublishAgent(
				context.Background(),
				connect.NewRequest(&registryv1.PublishAgentRequest{
					Payload: &registryv1.SignedPayload{
						Payload:   payload,
						Signature: sig,
						KeyId:     creds.PublisherID,
					},
				}),
			)
			if err != nil {
				return fmt.Errorf("publishing agent: %w", err)
			}

			fmt.Printf("agent published\n")
			fmt.Printf("id     : %s\n", resp.Msg.AgentId)
			fmt.Printf("name   : %s\n", resp.Msg.Agent.Name)
			fmt.Printf("version: %s\n", resp.Msg.Agent.Version)
			return nil
		},
	}

	cmd.Flags().StringVarP(&agentFile, "file", "f", "", "Path to agent JSON file")
	cmd.Flags().StringVar(&serverURL, "server", client.DefaultServerURL, "Registry server URL")

	return cmd
}
