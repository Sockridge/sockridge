package middleware

import (
	"context"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	"github.com/utsav-develops/SocialAgents/server/internal/auth"
)

type contextKey string

const PublisherIDKey contextKey = "publisher_id"

// NewAuthInterceptor returns a connect interceptor that validates the
// session token on protected RPCs. Unprotected RPCs (auth + discovery) pass through.
func NewAuthInterceptor(authSvc *auth.Service) connect.UnaryInterceptorFunc {
	protected := map[string]bool{
		"/agentregistry.v1.RegistryService/PublishAgent":   true,
		"/agentregistry.v1.RegistryService/UpdateAgent":    true,
		"/agentregistry.v1.RegistryService/DeprecateAgent": true,
		"/agentregistry.v1.RegistryService/GetAgent":       true,
	}

	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			procedure := req.Spec().Procedure

			if !protected[procedure] {
				return next(ctx, req)
			}

			token := extractBearer(req.Header())
			if token == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, nil)
			}

			publisherID, err := authSvc.ValidateJWT(token)
			if err != nil {
				return nil, connect.NewError(connect.CodeUnauthenticated, err)
			}

			ctx = context.WithValue(ctx, PublisherIDKey, publisherID)
			return next(ctx, req)
		}
	}
}

func extractBearer(h http.Header) string {
	v := h.Get("Authorization")
	if !strings.HasPrefix(v, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(v, "Bearer ")
}

// PublisherIDFromCtx retrieves the authenticated publisher ID from context.
func PublisherIDFromCtx(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(PublisherIDKey).(string)
	return id, ok
}
