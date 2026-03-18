package client

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"

	registryv1connect "github.com/utsav-develops/SocialAgents/server/gen/go/agentregistry/v1/registryv1connect"
)

const DefaultServerURL = "http://localhost:9000"

type Client struct {
	Registry  registryv1connect.RegistryServiceClient
	Discovery registryv1connect.DiscoveryServiceClient
	Access    registryv1connect.AccessAgreementServiceClient
}

func New(serverURL string, token string) *Client {
	opts := []connect.ClientOption{}
	if token != "" {
		opts = append(opts, connect.WithInterceptors(bearerInterceptor(token)))
	}

	httpClient := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
	}

	return &Client{
		Registry:  registryv1connect.NewRegistryServiceClient(httpClient, serverURL, opts...),
		Discovery: registryv1connect.NewDiscoveryServiceClient(httpClient, serverURL),
		Access:    registryv1connect.NewAccessAgreementServiceClient(httpClient, serverURL, opts...),
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
