package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/instantf/bot-x/shared/config"
	"github.com/instantf/bot-x/shared/database"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	pb "github.com/instantf/bot-x/gen/go/compliance/v1"
	"github.com/instantf/bot-x/services/compliance/internal/handler"
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

	// Resolve OFAC XML path (check standard locations)
	sdnPath := ""
	pathsToCheck := []string{
		"./shared/ofac/testdata/sdn.xml",
		"../shared/ofac/testdata/sdn.xml",
		"/app/shared/ofac/testdata/sdn.xml",
	}
	for _, p := range pathsToCheck {
		absPath, err := filepath.Abs(p)
		if err == nil {
			if _, err := os.Stat(absPath); err == nil {
				sdnPath = absPath
				logger.Info("found OFAC SDN file", zap.String("path", absPath))
				break
			}
		}
	}

	// Parse listen port
	parts := strings.Split(cfg.GRPCComplianceAddr, ":")
	port := ":" + parts[len(parts)-1]

	lis, err := net.Listen("tcp", port)
	if err != nil {
		logger.Fatal("failed to listen", zap.String("port", port), zap.Error(err))
	}

	grpcServer := grpc.NewServer()
	pb.RegisterComplianceServiceServer(grpcServer, handler.NewComplianceHandler(pool, sdnPath))

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		logger.Info("shutting down gRPC compliance server...")
		grpcServer.GracefulStop()
	}()

	logger.Info("starting gRPC compliance service", zap.String("addr", cfg.GRPCComplianceAddr))
	if err := grpcServer.Serve(lis); err != nil {
		logger.Fatal("failed to serve", zap.Error(err))
	}
}
