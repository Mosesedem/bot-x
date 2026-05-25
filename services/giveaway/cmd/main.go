package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/instantf/bot-x/shared/config"
	"github.com/instantf/bot-x/shared/database"
	"github.com/instantf/bot-x/shared/grpcdial"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	pbAudit "github.com/instantf/bot-x/gen/go/audit/v1"
	pbEntry "github.com/instantf/bot-x/gen/go/entry/v1"
	pb "github.com/instantf/bot-x/gen/go/giveaway/v1"
	pbKYC "github.com/instantf/bot-x/gen/go/kyc/v1"
	pbNotification "github.com/instantf/bot-x/gen/go/notification/v1"
	pbPayment "github.com/instantf/bot-x/gen/go/payment/v1"
	"github.com/instantf/bot-x/services/giveaway/internal/handler"
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
	entryConn := dialService(cfg.GRPCEntryAddr, "Entry", logger)
	defer entryConn.Close()
	entryClient := pbEntry.NewEntryServiceClient(entryConn)

	kycConn := dialService(cfg.GRPCKYCAddr, "KYC", logger)
	defer kycConn.Close()
	kycClient := pbKYC.NewKYCServiceClient(kycConn)

	notificationConn := dialService(cfg.GRPCNotificationAddr, "Notification", logger)
	defer notificationConn.Close()
	notificationClient := pbNotification.NewNotificationServiceClient(notificationConn)

	paymentConn := dialService(cfg.GRPCPaymentRouterAddr, "Payment", logger)
	defer paymentConn.Close()
	paymentClient := pbPayment.NewPaymentRouterServiceClient(paymentConn)

	auditConn := dialService(cfg.GRPCAuditAddr, "Audit", logger)
	defer auditConn.Close()
	auditClient := pbAudit.NewAuditServiceClient(auditConn)

	// Initialize Giveaway Service Handlers
	giveawayHandler := handler.NewGiveawayHandler(pool, entryClient, kycClient, notificationClient, paymentClient, auditClient, cfg)

	// ── Start gRPC Server ──
	parts := strings.Split(cfg.GRPCGiveawayAddr, ":")
	grpcPort := ":" + parts[len(parts)-1]

	grpcLis, err := net.Listen("tcp", grpcPort)
	if err != nil {
		logger.Fatal("failed to listen on gRPC port", zap.String("port", grpcPort), zap.Error(err))
	}

	grpcServer := grpc.NewServer()
	pb.RegisterGiveawayServiceServer(grpcServer, giveawayHandler)

	go func() {
		logger.Info("starting gRPC giveaway service", zap.String("addr", cfg.GRPCGiveawayAddr))
		if err := grpcServer.Serve(grpcLis); err != nil {
			logger.Fatal("failed to serve gRPC", zap.Error(err))
		}
	}()

	// ── Start HTTP REST Server ──
	r := chi.NewRouter()
	giveawayHandler.RegisterRoutes(r)

	httpPort := ":8080" // maps to 8082 in docker-compose
	httpServer := &http.Server{
		Addr:    httpPort,
		Handler: r,
	}

	go func() {
		logger.Info("starting HTTP REST server", zap.String("port", httpPort))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("failed to serve HTTP", zap.Error(err))
		}
	}()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down giveaway service...")
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
