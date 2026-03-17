package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/utsav-develops/SocialAgents/server/gen/go/agentregistry/v1/registryv1connect"
	"github.com/utsav-develops/SocialAgents/server/internal/auth"
	"github.com/utsav-develops/SocialAgents/server/internal/config"
	"github.com/utsav-develops/SocialAgents/server/internal/discovery"
	"github.com/utsav-develops/SocialAgents/server/internal/embedder"
	"github.com/utsav-develops/SocialAgents/server/internal/registry"
	"github.com/utsav-develops/SocialAgents/server/internal/store"
	"github.com/utsav-develops/SocialAgents/server/middleware"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	// ── Storage ───────────────────────────────────────────────────────────────
	scyllaStore, err := store.NewScyllaStore(cfg.Scylla)
	if err != nil {
		logger.Error("failed to connect to scylla", "err", err)
		os.Exit(1)
	}
	defer scyllaStore.Close()

	if err := scyllaStore.CreateSchema(context.Background(), cfg.Scylla.Keyspace); err != nil {
		logger.Error("failed to create scylla schema", "err", err)
		os.Exit(1)
	}

	redisStore, err := store.NewRedisStore(cfg.Redis)
	if err != nil {
		logger.Error("failed to connect to redis", "err", err)
		os.Exit(1)
	}
	defer redisStore.Close()

	vectorStore, err := store.NewVectorStore(cfg.Postgres)
	if err != nil {
		logger.Error("failed to connect to postgres/pgvector", "err", err)
		os.Exit(1)
	}
	defer vectorStore.Close()

	// ── Embedder sidecar ──────────────────────────────────────────────────────
	embedderClient := embedder.New(cfg.Embedder.URL)
	logger.Info("embedder configured", "url", cfg.Embedder.URL)

	// ── Services ──────────────────────────────────────────────────────────────
	authSvc := auth.New(&cfg.Auth, redisStore)
	registrySvc := registry.New(scyllaStore, scyllaStore, redisStore, vectorStore, authSvc, embedderClient)
	discoverySvc := discovery.New(scyllaStore, redisStore, vectorStore, embedderClient)

	// ── Interceptors ─────────────────────────────────────────────────────────
	interceptors := connect.WithInterceptors(
		middleware.NewAuthInterceptor(authSvc),
	)

	// ── Routes ───────────────────────────────────────────────────────────────
	mux := http.NewServeMux()
	mux.Handle(registryv1connect.NewRegistryServiceHandler(registrySvc, interceptors))
	mux.Handle(registryv1connect.NewDiscoveryServiceHandler(discoverySvc))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	addr := fmt.Sprintf(":%d", cfg.Server.GRPCPort)
	logger.Info("server starting", "addr", addr, "env", cfg.Server.Env)

	if err := http.ListenAndServe(addr, h2c.NewHandler(mux, &http2.Server{})); err != nil {
		logger.Error("server exited", "err", err)
		os.Exit(1)
	}
}
