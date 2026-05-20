package config

import (
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	DB       DBConfig
	SystemC  SystemCConfig
	Worker   WorkerConfig
	Webhook  WebhookConfig
	Security SecurityConfig
	CB       CircuitBreakerConfig
	LogLevel string
}

type ServerConfig struct {
	Port     int
	GRPCPort int
}

type DBConfig struct {
	Host         string
	Port         int
	Name         string
	User         string
	Password     string
	MaxOpenConns int
	MaxIdleConns int
}

type SystemCConfig struct {
	BaseURL            string
	APIKey             string
	TimeoutSeconds     int
	RateLimitPerSecond int
}

type WorkerConfig struct {
	Count            int
	PollIntervalSecs int
	ItemsPerPoll     int
}

type WebhookConfig struct {
	MaxRetries  int
	SigningAlgo string
}

type SecurityConfig struct {
	APIKeyHeader           string
	ValidAPIKeys           []string
	TimestampToleranceSecs int
}

type CircuitBreakerConfig struct {
	MaxRequestsHalfOpen int
	OpenTimeoutSeconds  int
}

func Load() (*Config, error) {
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Set defaults
	viper.SetDefault("SERVER_PORT", 8080)
	viper.SetDefault("GRPC_PORT", 9090)
	viper.SetDefault("WORKER_COUNT", 10)
	viper.SetDefault("POLL_INTERVAL_SECONDS", 2)
	viper.SetDefault("ITEMS_PER_POLL", 50)
	viper.SetDefault("DB_MAX_OPEN_CONNS", 20)
	viper.SetDefault("DB_MAX_IDLE_CONNS", 5)
	viper.SetDefault("SYSTEM_C_TIMEOUT_SECONDS", 15)
	viper.SetDefault("SYSTEM_C_RATE_LIMIT_PER_SECOND", 100)
	viper.SetDefault("CB_MAX_REQUESTS_HALF_OPEN", 5)
	viper.SetDefault("CB_OPEN_TIMEOUT_SECONDS", 60)
	viper.SetDefault("WEBHOOK_MAX_RETRIES", 5)
	viper.SetDefault("WEBHOOK_SIGNING_ALGORITHM", "sha256")
	viper.SetDefault("TIMESTAMP_TOLERANCE_SECONDS", 300)
	viper.SetDefault("LOG_LEVEL", "info")

	// Read .env if exists (ignoring error if it doesn't)
	viper.SetConfigFile(".env")
	_ = viper.ReadInConfig()

	cfg := &Config{
		Server: ServerConfig{
			Port:     viper.GetInt("SERVER_PORT"),
			GRPCPort: viper.GetInt("GRPC_PORT"),
		},
		DB: DBConfig{
			Host:         viper.GetString("DB_HOST"),
			Port:         viper.GetInt("DB_PORT"),
			Name:         viper.GetString("DB_NAME"),
			User:         viper.GetString("DB_USER"),
			Password:     viper.GetString("DB_PASSWORD"),
			MaxOpenConns: viper.GetInt("DB_MAX_OPEN_CONNS"),
			MaxIdleConns: viper.GetInt("DB_MAX_IDLE_CONNS"),
		},
		SystemC: SystemCConfig{
			BaseURL:            viper.GetString("SYSTEM_C_BASE_URL"),
			APIKey:             viper.GetString("SYSTEM_C_API_KEY"),
			TimeoutSeconds:     viper.GetInt("SYSTEM_C_TIMEOUT_SECONDS"),
			RateLimitPerSecond: viper.GetInt("SYSTEM_C_RATE_LIMIT_PER_SECOND"),
		},
		Worker: WorkerConfig{
			Count:            viper.GetInt("WORKER_COUNT"),
			PollIntervalSecs: viper.GetInt("POLL_INTERVAL_SECONDS"),
			ItemsPerPoll:     viper.GetInt("ITEMS_PER_POLL"),
		},
		Webhook: WebhookConfig{
			MaxRetries:  viper.GetInt("WEBHOOK_MAX_RETRIES"),
			SigningAlgo: viper.GetString("WEBHOOK_SIGNING_ALGORITHM"),
		},
		Security: SecurityConfig{
			APIKeyHeader:           viper.GetString("API_KEY_HEADER"),
			ValidAPIKeys:           viper.GetStringSlice("VALID_API_KEYS"),
			TimestampToleranceSecs: viper.GetInt("TIMESTAMP_TOLERANCE_SECONDS"),
		},
		CB: CircuitBreakerConfig{
			MaxRequestsHalfOpen: viper.GetInt("CB_MAX_REQUESTS_HALF_OPEN"),
			OpenTimeoutSeconds:  viper.GetInt("CB_OPEN_TIMEOUT_SECONDS"),
		},
		LogLevel: viper.GetString("LOG_LEVEL"),
	}

	return cfg, nil
}
