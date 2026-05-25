package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/instantf/bot-x/shared/config"
	"github.com/instantf/bot-x/shared/database"
	"github.com/instantf/bot-x/shared/gateways/safehaven"
	"github.com/instantf/bot-x/shared/grpcdial"
	"github.com/instantf/bot-x/shared/vault"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	pbAudit "github.com/instantf/bot-x/gen/go/audit/v1"
	pbCompliance "github.com/instantf/bot-x/gen/go/compliance/v1"
	pbGiveaway "github.com/instantf/bot-x/gen/go/giveaway/v1"
	pbNotification "github.com/instantf/bot-x/gen/go/notification/v1"
	pb "github.com/instantf/bot-x/gen/go/payment/v1"
	"github.com/instantf/bot-x/services/payment-router/internal/handler"
	"github.com/instantf/bot-x/services/payment-router/internal/router"
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

	// Dial external gRPC Services
	complianceConn := dialService(cfg.GRPCComplianceAddr, "Compliance", logger)
	defer complianceConn.Close()
	complianceClient := pbCompliance.NewComplianceServiceClient(complianceConn)

	auditConn := dialService(cfg.GRPCAuditAddr, "Audit", logger)
	defer auditConn.Close()
	auditClient := pbAudit.NewAuditServiceClient(auditConn)

	giveawayConn := dialService(cfg.GRPCGiveawayAddr, "Giveaway", logger)
	defer giveawayConn.Close()
	giveawayClient := pbGiveaway.NewGiveawayServiceClient(giveawayConn)

	notificationConn := dialService(cfg.GRPCNotificationAddr, "Notification", logger)
	defer notificationConn.Close()
	notificationClient := pbNotification.NewNotificationServiceClient(notificationConn)

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

	// ── Start gRPC Server ──
	parts := strings.Split(cfg.GRPCPaymentRouterAddr, ":")
	grpcPort := ":" + parts[len(parts)-1]

	grpcLis, err := net.Listen("tcp", grpcPort)
	if err != nil {
		logger.Fatal("failed to listen on gRPC port", zap.String("port", grpcPort), zap.Error(err))
	}

	grpcServer := grpc.NewServer()
	paymentRouterSvc := router.NewPaymentRouter(pool, shClient, complianceClient, auditClient, giveawayClient, "")
	pb.RegisterPaymentRouterServiceServer(grpcServer, paymentRouterSvc)

	go func() {
		logger.Info("starting gRPC payment-router service", zap.String("addr", cfg.GRPCPaymentRouterAddr))
		if err := grpcServer.Serve(grpcLis); err != nil {
			logger.Fatal("failed to serve gRPC", zap.Error(err))
		}
	}()

	// ── Start HTTP Webhook Server ──
	r := chi.NewRouter()
	webhookHandler := handler.NewWebhookHandler(pool, giveawayClient, notificationClient, cfg, logger)
	webhookHandler.RegisterRoutes(r)

	httpPort := ":8080" // bound to container port 8080 (maps to 8084 in docker-compose)
	httpServer := &http.Server{
		Addr:    httpPort,
		Handler: r,
	}

	go func() {
		logger.Info("starting HTTP webhook listener", zap.String("port", httpPort))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("failed to serve HTTP", zap.Error(err))
		}
	}()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down payment-router server...")
	grpcServer.GracefulStop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("failed to shutdown HTTP server gracefully", zap.Error(err))
	}
}

func dialService(addr, name string, logger *zap.Logger) *grpc.ClientConn {
	var conn *grpc.ClientConn
	var err error
	creds, credErr := grpcdial.TransportCredentials("", addr)
	if credErr != nil {
		logger.Fatal("failed to configure grpc transport credentials", zap.String("service", name), zap.Error(credErr))
	}
	for i := 0; i < 5; i++ {
		conn, err = grpc.Dial(
			addr,
			grpc.WithTransportCredentials(creds),
			grpc.WithBlock(),
		)
		if err == nil {
			return conn
		}
		logger.Warn("failed to connect to gRPC service, retrying...", zap.String("service", name), zap.Int("attempt", i+1), zap.Error(err))
		time.Sleep(2 * time.Second)
	}
	logger.Fatal("failed to connect to gRPC service after retries", zap.String("service", name), zap.String("addr", addr), zap.Error(err))
	return nil
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
