package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/joho/godotenv"
	"github.com/mosesedem/bot-x/shared/vault"
	"github.com/spf13/viper"
)

type Config struct {
	AppEnv                        string  `mapstructure:"APP_ENV"`
	LogLevel                      string  `mapstructure:"LOG_LEVEL"`
	BaseURL                       string  `mapstructure:"BASE_URL"`
	DatabaseURL                   string  `mapstructure:"DATABASE_URL"`
	RedisURL                      string  `mapstructure:"REDIS_URL"`
	ClickHouseURL                 string  `mapstructure:"CLICKHOUSE_URL"`
	ClickHouseDB                  string  `mapstructure:"CLICKHOUSE_DB"`
	VaultAddr                     string  `mapstructure:"VAULT_ADDR"`
	VaultToken                    string  `mapstructure:"VAULT_TOKEN"`
	XConsumerKey                  string  `mapstructure:"X_CONSUMER_KEY"`
	XConsumerSecret               string  `mapstructure:"X_CONSUMER_SECRET"`
	XAccessToken                  string  `mapstructure:"X_ACCESS_TOKEN"`
	XAccessSecret                 string  `mapstructure:"X_ACCESS_SECRET"`
	XWebhookEnv                   string  `mapstructure:"X_WEBHOOK_ENV"`
	XBearerToken                  string  `mapstructure:"X_BEARER_TOKEN"`
	XWebhookSecret                string  `mapstructure:"X_WEBHOOK_SECRET"`
	BotTwitterID                  string  `mapstructure:"BOT_TWITTER_ID"`
	BotTwitterHandle              string  `mapstructure:"BOT_TWITTER_HANDLE"`
	SafeHavenBaseURL              string  `mapstructure:"SAFEHAVEN_BASE_URL"`
	SafeHavenClientID             string  `mapstructure:"SAFEHAVEN_CLIENT_ID"`
	SafeHavenClientSecret         string  `mapstructure:"SAFEHAVEN_CLIENT_SECRET"`
	SafeHavenPrivateKeyPath       string  `mapstructure:"SAFEHAVEN_PRIVATE_KEY_PATH"`
	SafeHavenPrivateKeyPEM        string  `mapstructure:"SAFEHAVEN_PRIVATE_KEY_PEM"`
	FlutterwaveSecretKey          string  `mapstructure:"FLUTTERWAVE_SECRET_KEY"`
	FlutterwaveHashSecret         string  `mapstructure:"FLUTTERWAVE_HASH_SECRET"`
	PaystackSecretKey             string  `mapstructure:"PAYSTACK_SECRET_KEY"`
	PaystackWebhookSecret         string  `mapstructure:"PAYSTACK_WEBHOOK_SECRET"`
	StripeSecretKey               string  `mapstructure:"STRIPE_SECRET_KEY"`
	StripeWebhookSecret           string  `mapstructure:"STRIPE_WEBHOOK_SECRET"`
	StripePlatformAccountID       string  `mapstructure:"STRIPE_PLATFORM_ACCOUNT_ID"`
	CryptoRPCURL                  string  `mapstructure:"CRYPTO_RPC_URL"`
	CryptoChainID                 int     `mapstructure:"CRYPTO_CHAIN_ID"`
	CryptoTreasuryVaultPath       string  `mapstructure:"CRYPTO_TREASURY_VAULT_PATH"`
	AdminJWTSecret                string  `mapstructure:"ADMIN_JWT_SECRET"`
	AdminDualApprovalThresholdNGN float64 `mapstructure:"ADMIN_DUAL_APPROVAL_THRESHOLD_NGN"`
	AdminDualApprovalThresholdUSD float64 `mapstructure:"ADMIN_DUAL_APPROVAL_THRESHOLD_USD"`
	GRPCXGatewayAddr              string  `mapstructure:"GRPC_XGATEWAY_ADDR"`
	GRPCGiveawayAddr              string  `mapstructure:"GRPC_GIVEAWAY_ADDR"`
	GRPCEntryAddr                 string  `mapstructure:"GRPC_ENTRY_ADDR"`
	GRPCPaymentRouterAddr         string  `mapstructure:"GRPC_PAYMENT_ROUTER_ADDR"`
	GRPCKYCAddr                   string  `mapstructure:"GRPC_KYC_ADDR"`
	GRPCComplianceAddr            string  `mapstructure:"GRPC_COMPLIANCE_ADDR"`
	GRPCAuditAddr                 string  `mapstructure:"GRPC_AUDIT_ADDR"`
	GRPCNotificationAddr          string  `mapstructure:"GRPC_NOTIFICATION_ADDR"`
	GRPCReconciliationAddr        string  `mapstructure:"GRPC_RECONCILIATION_ADDR"`
	PagerDutyRoutingKey           string  `mapstructure:"PAGERDUTY_ROUTING_KEY"`
	SlackWebhookURL               string  `mapstructure:"SLACK_WEBHOOK_URL"`
	RateLimitWebhookRPS           int     `mapstructure:"RATE_LIMIT_WEBHOOK_RPS"`
	RateLimitAPIRPS               int     `mapstructure:"RATE_LIMIT_API_RPS"`
	MaxActiveGiveawaysPerHost     int     `mapstructure:"MAX_ACTIVE_GIVEAWAYS_PER_HOST"`
}

func Load() (*Config, error) {
	// Load .env file if it exists (for local development)
	_ = godotenv.Load()

	viper.AutomaticEnv()
	// Replace dot with underscore for env variables
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Set Defaults
	viper.SetDefault("APP_ENV", "development")
	viper.SetDefault("LOG_LEVEL", "info")
	viper.SetDefault("BASE_URL", "http://localhost:8080")
	// DATABASE_URL must be set explicitly via environment — no hardcoded default
	viper.SetDefault("REDIS_URL", "redis://localhost:6379")
	viper.SetDefault("CLICKHOUSE_URL", "clickhouse://localhost:9000")
	viper.SetDefault("CLICKHOUSE_DB", "instantf_audit")
	viper.SetDefault("VAULT_ADDR", "http://localhost:8200")
	// VAULT_TOKEN must be set explicitly — no default token in production
	viper.SetDefault("X_WEBHOOK_ENV", "dev")
	viper.SetDefault("BOT_TWITTER_HANDLE", "instantf_bot")
	viper.SetDefault("SAFEHAVEN_BASE_URL", "https://api.safehavenmfb.com")
	viper.SetDefault("CRYPTO_CHAIN_ID", 8453)
	viper.SetDefault("CRYPTO_TREASURY_VAULT_PATH", "secret/crypto/treasury")
	viper.SetDefault("ADMIN_DUAL_APPROVAL_THRESHOLD_NGN", 500000.0)
	viper.SetDefault("ADMIN_DUAL_APPROVAL_THRESHOLD_USD", 1000.0)
	// gRPC service addresses — must be explicitly configured per environment
	viper.SetDefault("GRPC_XGATEWAY_ADDR", "localhost:50051")
	viper.SetDefault("GRPC_GIVEAWAY_ADDR", "localhost:50052")
	viper.SetDefault("GRPC_ENTRY_ADDR", "localhost:50053")
	viper.SetDefault("GRPC_PAYMENT_ROUTER_ADDR", "localhost:50054")
	viper.SetDefault("GRPC_KYC_ADDR", "localhost:50055")
	viper.SetDefault("GRPC_COMPLIANCE_ADDR", "localhost:50056")
	viper.SetDefault("GRPC_AUDIT_ADDR", "localhost:50057")
	viper.SetDefault("GRPC_NOTIFICATION_ADDR", "localhost:50058")
	viper.SetDefault("GRPC_RECONCILIATION_ADDR", "localhost:50059")
	viper.SetDefault("RATE_LIMIT_WEBHOOK_RPS", 1000)
	viper.SetDefault("RATE_LIMIT_API_RPS", 100)
	viper.SetDefault("MAX_ACTIVE_GIVEAWAYS_PER_HOST", 5)

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// If Vault is configured, try to load sensitive credentials from it.
	if cfg.VaultAddr != "" && cfg.VaultToken != "" {
		ctx := context.Background()
		vclient, err := vault.New(cfg.VaultAddr, cfg.VaultToken)
		if err != nil {
			// In production we require Vault to be available.
			if cfg.AppEnv == "production" {
				return nil, fmt.Errorf("failed to initialize Vault client: %w", err)
			}
		} else {
			// Try to read X/Twitter consumer secrets from secret/x
			if val, err := vclient.GetSecretString(ctx, "x", "consumer_secret"); err == nil && val != "" {
				cfg.XConsumerSecret = val
			}
			if val, err := vclient.GetSecretString(ctx, "x", "consumer_key"); err == nil && val != "" {
				cfg.XConsumerKey = val
			}

			// Try to read SafeHaven credentials from secret/safehaven
			if val, err := vclient.GetSecretString(ctx, "safehaven", "client_secret"); err == nil && val != "" {
				cfg.SafeHavenClientSecret = val
			}
			if val, err := vclient.GetSecretString(ctx, "safehaven", "client_id"); err == nil && val != "" {
				cfg.SafeHavenClientID = val
			}

			// Add other secret reads here as needed (stripe, paystack, flutterwave...)
		}
	}

	return &cfg, nil
}
