package main

import (
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/instantf/bot-x/shared/config"
	"github.com/instantf/bot-x/shared/grpcdial"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	pb "github.com/instantf/bot-x/gen/go/notification/v1"
	pbX "github.com/instantf/bot-x/gen/go/xgateway/v1"
	"github.com/instantf/bot-x/services/notification/internal/handler"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	// Dial X Gateway Service
	var conn *grpc.ClientConn
	creds, credErr := grpcdial.TransportCredentials("", cfg.GRPCXGatewayAddr)
	if credErr != nil {
		logger.Fatal("failed to configure grpc transport credentials", zap.Error(credErr))
	}
	for i := 0; i < 5; i++ {
		conn, err = grpc.Dial(
			cfg.GRPCXGatewayAddr,
			grpc.WithTransportCredentials(creds),
			grpc.WithBlock(),
		)
		if err == nil {
			break
		}
		logger.Warn("failed to connect to X Gateway, retrying...", zap.Int("attempt", i+1), zap.Error(err))
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		logger.Fatal("failed to connect to X Gateway after retries", zap.Error(err))
	}
	defer conn.Close()

	xGatewayClient := pbX.NewXGatewayServiceClient(conn)

	// Parse listen port
	parts := strings.Split(cfg.GRPCNotificationAddr, ":")
	port := ":" + parts[len(parts)-1]

	lis, err := net.Listen("tcp", port)
	if err != nil {
		logger.Fatal("failed to listen", zap.String("port", port), zap.Error(err))
	}

	grpcServer := grpc.NewServer()
	pb.RegisterNotificationServiceServer(grpcServer, handler.NewNotificationHandler(xGatewayClient))

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		logger.Info("shutting down gRPC notification server...")
		grpcServer.GracefulStop()
	}()

	logger.Info("starting gRPC notification service", zap.String("addr", cfg.GRPCNotificationAddr))
	if err := grpcServer.Serve(lis); err != nil {
		logger.Fatal("failed to serve", zap.Error(err))
	}
}
