package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mosesedem/bot-x/shared/config"
	"github.com/mosesedem/bot-x/shared/database"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	pb "github.com/mosesedem/bot-x/gen/go/entry/v1"
	"github.com/mosesedem/bot-x/services/entry/internal/handler"
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

	// Parse listen port
	parts := strings.Split(cfg.GRPCEntryAddr, ":")
	port := ":" + parts[len(parts)-1]

	lis, err := net.Listen("tcp", port)
	if err != nil {
		logger.Fatal("failed to listen", zap.String("port", port), zap.Error(err))
	}

	grpcServer := grpc.NewServer()
	pb.RegisterEntryServiceServer(grpcServer, handler.NewEntryHandler(pool))

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		logger.Info("shutting down gRPC entry server...")
		grpcServer.GracefulStop()
	}()

	logger.Info("starting gRPC entry service", zap.String("addr", cfg.GRPCEntryAddr))
	if err := grpcServer.Serve(lis); err != nil {
		logger.Fatal("failed to serve", zap.Error(err))
	}
}
