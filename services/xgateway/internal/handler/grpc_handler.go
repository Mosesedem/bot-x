package handler

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	pb "github.com/mosesedem/bot-x/gen/go/xgateway/v1"
	"github.com/mosesedem/bot-x/shared/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type XGatewayGRPCHandler struct {
	pb.UnimplementedXGatewayServiceServer
	db     *pgxpool.Pool
	cfg    *config.Config
	logger *zap.Logger
}

func NewXGatewayGRPCHandler(db *pgxpool.Pool, cfg *config.Config, logger *zap.Logger) *XGatewayGRPCHandler {
	return &XGatewayGRPCHandler{
		db:     db,
		cfg:    cfg,
		logger: logger,
	}
}

func (h *XGatewayGRPCHandler) SendDM(ctx context.Context, req *pb.SendDMRequest) (*pb.SendDMResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "X API integration pending for SendDM")
}

func (h *XGatewayGRPCHandler) ReplyToTweet(ctx context.Context, req *pb.ReplyToTweetRequest) (*pb.ReplyToTweetResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "X API integration pending for ReplyToTweet")
}

func (h *XGatewayGRPCHandler) GetTweetReplies(ctx context.Context, req *pb.GetTweetRepliesRequest) (*pb.GetTweetRepliesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "X API integration pending for GetTweetReplies")
}

func (h *XGatewayGRPCHandler) GetRetweeters(ctx context.Context, req *pb.GetRetweeterRequest) (*pb.GetRetweeterResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "X API integration pending for GetRetweeters")
}

func (h *XGatewayGRPCHandler) CheckFollows(ctx context.Context, req *pb.CheckFollowsRequest) (*pb.CheckFollowsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "X API integration pending for CheckFollows")
}

func (h *XGatewayGRPCHandler) GetUserProfile(ctx context.Context, req *pb.GetUserProfileRequest) (*pb.UserProfile, error) {
	return nil, status.Errorf(codes.Unimplemented, "X API integration pending for GetUserProfile")
}

func (h *XGatewayGRPCHandler) ParseTweetCommand(ctx context.Context, req *pb.ParseTweetCommandRequest) (*pb.GiveawayCommand, error) {
	// Simple stub logic that parses basic amounts for now, until NLP model is properly hooked up
	return &pb.GiveawayCommand{
		WinnerCount: 1,
		TotalAmount: 100,
	}, nil
}
