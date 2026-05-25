package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/instantf/bot-x/shared/config"
	"github.com/instantf/bot-x/shared/database"
	"github.com/instantf/bot-x/shared/gateways/safehaven"
	"github.com/instantf/bot-x/shared/vault"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	pb "github.com/instantf/bot-x/gen/go/kyc/v1"
	"github.com/instantf/bot-x/services/kyc/internal/handler"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	// Connect to database
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer database.Close(pool)

	// Load Safe Haven Private Key
	privKey, err := loadPrivateKey(cfg, logger)
	if err != nil {
		logger.Fatal("failed to load Safe Haven private key", zap.Error(err))
	}

	// Initialize Safe Haven Client
	shClient := safehaven.New(safehaven.Config{
		BaseURL:      cfg.SafeHavenBaseURL,
		ClientID:     cfg.SafeHavenClientID,
		ClientSecret: cfg.SafeHavenClientSecret,
		PrivateKey:   privKey,
	})

	// Parse listen port
	parts := strings.Split(cfg.GRPCKYCAddr, ":")
	port := ":" + parts[len(parts)-1]

	lis, err := net.Listen("tcp", port)
	if err != nil {
		logger.Fatal("failed to listen", zap.String("port", port), zap.Error(err))
	}

	grpcServer := grpc.NewServer()
	pb.RegisterKYCServiceServer(grpcServer, handler.NewKYCHandler(pool, shClient))

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		logger.Info("shutting down gRPC KYC server...")
		grpcServer.GracefulStop()
	}()

	logger.Info("starting gRPC KYC service", zap.String("addr", cfg.GRPCKYCAddr))
	if err := grpcServer.Serve(lis); err != nil {
		logger.Fatal("failed to serve", zap.Error(err))
	}
}

func loadPrivateKey(cfg *config.Config, logger *zap.Logger) (*rsa.PrivateKey, error) {
	// Try loading from file first
	if cfg.SafeHavenPrivateKeyPath != "" {
		keyBytes, err := os.ReadFile(cfg.SafeHavenPrivateKeyPath)
		if err == nil {
			key, err := jwt.ParseRSAPrivateKeyFromPEM(keyBytes)
			if err == nil {
				return key, nil
			}
			logger.Warn("failed to parse private key from file", zap.Error(err))
		} else {
			logger.Warn("failed to read private key file", zap.String("path", cfg.SafeHavenPrivateKeyPath), zap.Error(err))
		}
	}

	// Try loading from Vault
	if cfg.VaultAddr != "" && cfg.VaultToken != "" {
		vaultClient, err := vault.New(cfg.VaultAddr, cfg.VaultToken)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			keyStr, err := vaultClient.GetSecretString(ctx, "safehaven", "private_key")
			if err == nil && keyStr != "" {
				key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(keyStr))
				if err == nil {
					return key, nil
				}
				logger.Warn("failed to parse private key from Vault secret", zap.Error(err))
			} else {
				logger.Warn("failed to read private key from Vault", zap.Error(err))
			}
		} else {
			logger.Warn("failed to initialize Vault client", zap.Error(err))
		}
	}

	// Fallback to generating a dummy private key for local development
	logger.Warn("no Safe Haven private key configured, generating a temporary dummy key for dev purposes")
	return rsa.GenerateKey(rand.Reader, 2048)
}
