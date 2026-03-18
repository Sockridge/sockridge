// Package socialagents provides a Go SDK for the SocialAgents agent registry.
//
// Usage:
//
//	registry := socialagents.New("http://localhost:9000")
//	if err := registry.Login("", ""); err != nil {
//	    log.Fatal(err)
//	}
//
//	published, err := registry.Publish(ctx, &socialagents.AgentCard{
//	    Name:        "My Agent",
//	    Description: "Does something useful",
//	    URL:         "https://my-agent.example.com",
//	    Skills: []socialagents.Skill{{
//	        ID: "do.thing", Name: "Do Thing",
//	        Description: "Does the thing", Tags: []string{"thing"},
//	    }},
//	})
package socialagents

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"

	registryv1 "github.com/utsav-develops/SocialAgents/server/gen/go/agentregistry/v1"
	"github.com/utsav-develops/SocialAgents/server/gen/go/agentregistry/v1/registryv1connect"
)

// Registry is the main SDK client.
type Registry struct {
	serverURL   string
	token       string
	publisherID string

	registry  registryv1connect.RegistryServiceClient
	discovery registryv1connect.DiscoveryServiceClient
	access    registryv1connect.AccessAgreementServiceClient
}

// New creates a new Registry client.
func New(serverURL string) *Registry {
	hc := h2cClient()
	return &Registry{
		serverURL: serverURL,
		registry:  registryv1connect.NewRegistryServiceClient(hc, serverURL),
		discovery: registryv1connect.NewDiscoveryServiceClient(hc, serverURL),
		access:    registryv1connect.NewAccessAgreementServiceClient(hc, serverURL),
	}
}

func h2cClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
	}
}

func (r *Registry) withAuth() []connect.ClientOption {
	return []connect.ClientOption{
		connect.WithInterceptors(bearerInterceptor(r.token)),
	}
}

func bearerInterceptor(token string) connect.UnaryInterceptorFunc {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set("Authorization", "Bearer "+token)
			return next(ctx, req)
		})
	})
}

func (r *Registry) authRegistry() registryv1connect.RegistryServiceClient {
	return registryv1connect.NewRegistryServiceClient(h2cClient(), r.serverURL, r.withAuth()...)
}

func (r *Registry) authAccess() registryv1connect.AccessAgreementServiceClient {
	return registryv1connect.NewAccessAgreementServiceClient(h2cClient(), r.serverURL, r.withAuth()...)
}

func toProto(card *AgentCard) *registryv1.AgentCard {
	skills := make([]*registryv1.Skill, len(card.Skills))
	for i, s := range card.Skills {
		skills[i] = &registryv1.Skill{
			Id: s.ID, Name: s.Name, Description: s.Description, Tags: s.Tags,
		}
	}

	p := &registryv1.AgentCard{
		Name:            card.Name,
		Description:     card.Description,
		Url:             card.URL,
		Version:         card.Version,
		ProtocolVersion: card.ProtocolVersion,
		Skills:          skills,
	}

	if card.Capabilities != nil {
		p.Capabilities = &registryv1.Capabilities{
			Streaming:         card.Capabilities.Streaming,
			PushNotifications: card.Capabilities.PushNotifications,
			MultiTurn:         card.Capabilities.MultiTurn,
			ToolUse:           card.Capabilities.ToolUse,
		}
	}
	return p
}

func fromProto(p *registryv1.AgentCard) *AgentCard {
	if p == nil {
		return nil
	}

	skills := make([]Skill, len(p.Skills))
	for i, s := range p.Skills {
		skills[i] = Skill{ID: s.Id, Name: s.Name, Description: s.Description, Tags: s.Tags}
	}

	card := &AgentCard{
		ID:              p.Id,
		Name:            p.Name,
		Description:     p.Description,
		URL:             p.Url,
		Version:         p.Version,
		ProtocolVersion: p.ProtocolVersion,
		PublisherID:     p.PublisherId,
		Status:          p.Status.String(),
		Skills:          skills,
	}

	if p.Capabilities != nil {
		card.Capabilities = &Capabilities{
			Streaming:         p.Capabilities.Streaming,
			PushNotifications: p.Capabilities.PushNotifications,
			MultiTurn:         p.Capabilities.MultiTurn,
			ToolUse:           p.Capabilities.ToolUse,
		}
	}

	if p.GatekeeperResult != nil {
		card.GatekeeperResult = &GatekeeperResult{
			Approved:        p.GatekeeperResult.Approved,
			ConfidenceScore: p.GatekeeperResult.ConfidenceScore,
			Reason:          p.GatekeeperResult.Reason,
			Reachable:       p.GatekeeperResult.Reachable,
			PingLatencyMs:   p.GatekeeperResult.PingLatencyMs,
		}
	}

	return card
}
