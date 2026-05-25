package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/instantf/bot-x/shared/grpcdial"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	pbAudit "github.com/instantf/bot-x/gen/go/audit/v1"
	pbCompliance "github.com/instantf/bot-x/gen/go/compliance/v1"
	pbGiveaway "github.com/instantf/bot-x/gen/go/giveaway/v1"
	pbKYC "github.com/instantf/bot-x/gen/go/kyc/v1"
	pbPayment "github.com/instantf/bot-x/gen/go/payment/v1"
	"github.com/instantf/bot-x/services/admin/internal/handler"
	"github.com/instantf/bot-x/shared/config"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	// Dial downstream gRPC services
	giveawayConn := dialService(cfg.GRPCGiveawayAddr, "Giveaway", logger)
	defer giveawayConn.Close()

	complianceConn := dialService(cfg.GRPCComplianceAddr, "Compliance", logger)
	defer complianceConn.Close()

	kycConn := dialService(cfg.GRPCKYCAddr, "KYC", logger)
	defer kycConn.Close()

	paymentConn := dialService(cfg.GRPCPaymentRouterAddr, "Payment", logger)
	defer paymentConn.Close()

	auditConn := dialService(cfg.GRPCAuditAddr, "Audit", logger)
	defer auditConn.Close()

	giveawayClient := pbGiveaway.NewGiveawayServiceClient(giveawayConn)
	complianceClient := pbCompliance.NewComplianceServiceClient(complianceConn)
	kycClient := pbKYC.NewKYCServiceClient(kycConn)
	paymentClient := pbPayment.NewPaymentRouterServiceClient(paymentConn)
	auditClient := pbAudit.NewAuditServiceClient(auditConn)

	adminHandler := handler.NewAdminHandler(
		giveawayClient,
		complianceClient,
		kycClient,
		paymentClient,
		auditClient,
		logger,
		cfg,
	)

	r := chi.NewRouter()
	adminHandler.RegisterRoutes(r)

	httpServer := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	go func() {
		logger.Info("starting Admin HTTP service", zap.String("port", ":8080"))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("failed to serve HTTP", zap.Error(err))
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down Admin service...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP shutdown error", zap.Error(err))
	}
}

func dialService(addr, name string, logger *zap.Logger) *grpc.ClientConn {
	creds, credErr := grpcdial.TransportCredentials("", addr)
	if credErr != nil {
		logger.Fatal("failed to configure grpc transport credentials", zap.String("service", name), zap.Error(credErr))
	}
	for i := 0; i < 5; i++ {
		conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(creds))
		if err == nil {
			return conn
		}
		logger.Warn("failed to dial service, retrying...", zap.String("service", name), zap.Int("attempt", i+1), zap.Error(err))
		time.Sleep(2 * time.Second)
	}
	conn, _ := grpc.Dial(addr, grpc.WithTransportCredentials(creds))
	return conn
}
