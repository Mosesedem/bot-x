package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/instantf/bot-x/shared/config"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	pb "github.com/instantf/bot-x/gen/go/audit/v1"
	"github.com/instantf/bot-x/services/audit/internal/handler"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	// Connect to ClickHouse
	opts, err := clickhouse.ParseDSN(cfg.ClickHouseURL)
	if err != nil {
		logger.Fatal("failed to parse ClickHouse DSN", zap.Error(err))
	}
	// Explicitly set the database from configuration
	opts.Auth.Database = cfg.ClickHouseDB

	var chConn clickhouse.Conn
	for i := 0; i < 5; i++ {
		chConn, err = clickhouse.Open(opts)
		if err == nil {
			// Ping to ensure connection is valid
			err = chConn.Ping(context.Background())
			if err == nil {
				break
			}
		}
		logger.Warn("failed to connect to ClickHouse, retrying...", zap.Int("attempt", i+1), zap.Error(err))
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		logger.Fatal("failed to connect to ClickHouse after retries", zap.Error(err))
	}
	defer chConn.Close()

	// Parse listen address (bind to all interfaces on the port)
	parts := strings.Split(cfg.GRPCAuditAddr, ":")
	port := ":" + parts[len(parts)-1]

	lis, err := net.Listen("tcp", port)
	if err != nil {
		logger.Fatal("failed to listen", zap.String("port", port), zap.Error(err))
	}

	grpcServer := grpc.NewServer()
	pb.RegisterAuditServiceServer(grpcServer, handler.NewAuditHandler(chConn))

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		logger.Info("shutting down gRPC server gracefully...")
		grpcServer.GracefulStop()
	}()

	logger.Info("starting gRPC audit service", zap.String("addr", cfg.GRPCAuditAddr))
	if err := grpcServer.Serve(lis); err != nil {
		logger.Fatal("failed to serve", zap.Error(err))
	}
}
