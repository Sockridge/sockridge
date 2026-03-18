package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server     ServerConfig
	Scylla     ScyllaConfig
	Redis      RedisConfig
	Postgres   PostgresConfig
	Auth       AuthConfig
	Embedder   EmbedderConfig
	Gatekeeper GatekeeperConfig
}

type ServerConfig struct {
	GRPCPort int    `mapstructure:"grpc_port"`
	HTTPPort int    `mapstructure:"http_port"`
	Env      string `mapstructure:"env"`
}

type ScyllaConfig struct {
	Hosts    []string `mapstructure:"hosts"`
	Keyspace string   `mapstructure:"keyspace"`
	Username string   `mapstructure:"username"`
	Password string   `mapstructure:"password"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type PostgresConfig struct {
	DSN string `mapstructure:"dsn"`
}

type EmbedderConfig struct {
	URL string `mapstructure:"url"`
}

type GatekeeperConfig struct {
	GroqKey string `mapstructure:"groq_key"`
}

type AuthConfig struct {
	JWTSecret    string `mapstructure:"jwt_secret"`
	JWTExpiry    int    `mapstructure:"jwt_expiry_hours"`
	NonceTTLSecs int    `mapstructure:"nonce_ttl_secs"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")

	viper.SetEnvPrefix("AGENTREGISTRY")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// defaults
	viper.SetDefault("server.grpc_port", 9000)
	viper.SetDefault("server.http_port", 8080)
	viper.SetDefault("server.env", "dev")
	viper.SetDefault("auth.jwt_expiry_hours", 1)
	viper.SetDefault("auth.nonce_ttl_secs", 30)
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("embedder.url", "http://embedder:8000")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	// manually read env vars that Viper struggles to map to nested structs
	if hosts := os.Getenv("AGENTREGISTRY_SCYLLA_HOSTS"); hosts != "" {
		cfg.Scylla.Hosts = strings.Split(hosts, ",")
	}
	if keyspace := os.Getenv("AGENTREGISTRY_SCYLLA_KEYSPACE"); keyspace != "" {
		cfg.Scylla.Keyspace = keyspace
	}
	if addr := os.Getenv("AGENTREGISTRY_REDIS_ADDR"); addr != "" {
		cfg.Redis.Addr = addr
	}
	if dsn := os.Getenv("AGENTREGISTRY_POSTGRES_DSN"); dsn != "" {
		cfg.Postgres.DSN = dsn
	}
	if secret := os.Getenv("AGENTREGISTRY_AUTH_JWT_SECRET"); secret != "" {
		cfg.Auth.JWTSecret = secret
	}
	if url := os.Getenv("AGENTREGISTRY_EMBEDDER_URL"); url != "" {
		cfg.Embedder.URL = url
	}
	if key := os.Getenv("AGENTREGISTRY_GATEKEEPER_GROQ_KEY"); key != "" {
		cfg.Gatekeeper.GroqKey = key
	}

	return &cfg, nil
}