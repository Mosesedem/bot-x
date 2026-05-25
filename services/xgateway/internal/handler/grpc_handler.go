package handler

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	pb "github.com/mosesedem/bot-x/gen/go/xgateway/v1"
	"github.com/mosesedem/bot-x/shared/config"
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
	h.logger.Info("mock SendDM", zap.String("user_id", req.UserId), zap.String("message", req.Message))
	return &pb.SendDMResponse{DmEventId: "mock_dm_event_id"}, nil
}

func (h *XGatewayGRPCHandler) ReplyToTweet(ctx context.Context, req *pb.ReplyToTweetRequest) (*pb.ReplyToTweetResponse, error) {
	h.logger.Info("mock ReplyToTweet", zap.String("tweet_id", req.TweetId), zap.String("text", req.Text))
	return &pb.ReplyToTweetResponse{ReplyTweetId: "mock_reply_tweet_id"}, nil
}

func (h *XGatewayGRPCHandler) GetTweetReplies(ctx context.Context, req *pb.GetTweetRepliesRequest) (*pb.GetTweetRepliesResponse, error) {
	return &pb.GetTweetRepliesResponse{Users: nil}, nil
}

func (h *XGatewayGRPCHandler) GetRetweeters(ctx context.Context, req *pb.GetRetweeterRequest) (*pb.GetRetweeterResponse, error) {
	return &pb.GetRetweeterResponse{Users: []*pb.TweetUser{}}, nil
}

func (h *XGatewayGRPCHandler) CheckFollows(ctx context.Context, req *pb.CheckFollowsRequest) (*pb.CheckFollowsResponse, error) {
	return &pb.CheckFollowsResponse{Follows: true}, nil
}

func (h *XGatewayGRPCHandler) GetUserProfile(ctx context.Context, req *pb.GetUserProfileRequest) (*pb.UserProfile, error) {
	return &pb.UserProfile{
		TwitterId:      req.TwitterId,
		Handle:         "mock_user",
		FollowerCount:  100,
		FollowingCount: 50,
		IsVerified:     true,
	}, nil
}

func (h *XGatewayGRPCHandler) ParseTweetCommand(ctx context.Context, req *pb.ParseTweetCommandRequest) (*pb.GiveawayCommand, error) {
	return &pb.GiveawayCommand{
		WinnerCount: 1,
		TotalAmount: 100,
	}, nil
}
