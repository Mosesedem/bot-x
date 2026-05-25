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
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/instantf/bot-x/shared/grpcdial"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	pbAudit "github.com/instantf/bot-x/gen/go/audit/v1"
	pbEntry "github.com/instantf/bot-x/gen/go/entry/v1"
	pbGiveaway "github.com/instantf/bot-x/gen/go/giveaway/v1"
	pbNotification "github.com/instantf/bot-x/gen/go/notification/v1"
	pbPayment "github.com/instantf/bot-x/gen/go/payment/v1"
	pb "github.com/instantf/bot-x/gen/go/xgateway/v1"
	"github.com/instantf/bot-x/shared/config"
	"github.com/instantf/bot-x/shared/database"
	"github.com/instantf/bot-x/services/xgateway/internal/handler"
	"github.com/instantf/bot-x/services/xgateway/internal/worker"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer database.Close(pool)

	// Asynq client (for webhook handler to enqueue jobs)
	redisOpt := asynq.RedisClientOpt{Addr: cfg.RedisURL}
	asynqClient := asynq.NewClient(redisOpt)
	defer asynqClient.Close()

	// Dial downstream gRPC services
	giveawayConn := dialService(cfg.GRPCGiveawayAddr, "Giveaway", logger)
	defer giveawayConn.Close()

	paymentConn := dialService(cfg.GRPCPaymentRouterAddr, "Payment", logger)
	defer paymentConn.Close()

	notificationConn := dialService(cfg.GRPCNotificationAddr, "Notification", logger)
	defer notificationConn.Close()

	entryConn := dialService(cfg.GRPCEntryAddr, "Entry", logger)
	defer entryConn.Close()

	auditConn := dialService(cfg.GRPCAuditAddr, "Audit", logger)
	defer auditConn.Close()

	giveawayClient := pbGiveaway.NewGiveawayServiceClient(giveawayConn)
	paymentClient := pbPayment.NewPaymentRouterServiceClient(paymentConn)
	notificationClient := pbNotification.NewNotificationServiceClient(notificationConn)
	entryClient := pbEntry.NewEntryServiceClient(entryConn)
	auditClient := pbAudit.NewAuditServiceClient(auditConn)

	// ── Asynq Worker ──
	eventWorker := worker.NewEventWorker(pool, giveawayClient, paymentClient, notificationClient, entryClient, auditClient, logger, cfg)
	asynqServer := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 10,
		Queues:      map[string]int{"default": 10},
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc("x_webhook_event", eventWorker.ProcessTask)

	go func() {
		logger.Info("starting Asynq worker")
		if err := asynqServer.Run(mux); err != nil {
			logger.Fatal("failed to run Asynq server", zap.Error(err))
		}
	}()

	// ── gRPC Server (XGatewayService) ──
	xGatewayHandler := handler.NewXGatewayGRPCHandler(pool, cfg, logger)

	parts := strings.Split(cfg.GRPCXGatewayAddr, ":")
	grpcPort := ":" + parts[len(parts)-1]
	grpcLis, err := net.Listen("tcp", grpcPort)
	if err != nil {
		logger.Fatal("failed to listen on gRPC port", zap.String("port", grpcPort), zap.Error(err))
	}

	grpcServer := grpc.NewServer()
	pb.RegisterXGatewayServiceServer(grpcServer, xGatewayHandler)

	go func() {
		logger.Info("starting gRPC X Gateway service", zap.String("addr", cfg.GRPCXGatewayAddr))
		if err := grpcServer.Serve(grpcLis); err != nil {
			logger.Fatal("failed to serve gRPC", zap.Error(err))
		}
	}()

	// ── HTTP Server (Webhook handler) ──
	webhookHandler := handler.NewXWebhookHandler(asynqClient, cfg, logger)
	r := chi.NewRouter()
	webhookHandler.RegisterRoutes(r)

	// Health/ready probes
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	r.Get("/ready", func(w http.ResponseWriter, req *http.Request) {
		if err := database.Ping(req.Context(), pool); err != nil {
			http.Error(w, `{"status":"not_ready"}`, http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	httpServer := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	go func() {
		logger.Info("starting HTTP webhook server", zap.String("port", ":8080"))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("failed to serve HTTP", zap.Error(err))
		}
	}()

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down X Gateway service...")
	grpcServer.GracefulStop()
	asynqServer.Shutdown()

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
	// Return a non-blocking connection (lazy dial)
	conn, _ := grpc.Dial(addr, grpc.WithTransportCredentials(creds))
	return conn
}

// Ensure pool is accessible via database helper
func init() {
	// Use pgxpool directly; database.Ping wraps pool.Ping
	_ = (*pgxpool.Pool)(nil)
}
