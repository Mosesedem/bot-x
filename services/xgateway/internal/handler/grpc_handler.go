package handler

import (
	"context"

	"go.uber.org/zap"

	pb "github.com/mosesedem/bot-x/gen/go/xgateway/v1"
	"github.com/mosesedem/bot-x/shared/gateways/xapi"
	"github.com/mosesedem/bot-x/shared/nlp/commandparser"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type XGatewayGRPCHandler struct {
	pb.UnimplementedXGatewayServiceServer
	xClient *xapi.Client
	logger  *zap.Logger
}

func NewXGatewayGRPCHandler(xClient *xapi.Client, logger *zap.Logger) *XGatewayGRPCHandler {
	return &XGatewayGRPCHandler{
		xClient: xClient,
		logger:  logger,
	}
}

func (h *XGatewayGRPCHandler) SendDM(ctx context.Context, req *pb.SendDMRequest) (*pb.SendDMResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	eventID, err := h.xClient.SendDM(ctx, req.UserId, req.Message)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "x send dm failed: %v", err)
	}
	return &pb.SendDMResponse{DmEventId: eventID}, nil
}

func (h *XGatewayGRPCHandler) ReplyToTweet(ctx context.Context, req *pb.ReplyToTweetRequest) (*pb.ReplyToTweetResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	replyID, err := h.xClient.ReplyToTweet(ctx, req.TweetId, req.Text)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "x reply failed: %v", err)
	}
	return &pb.ReplyToTweetResponse{ReplyTweetId: replyID}, nil
}

func (h *XGatewayGRPCHandler) GetTweetReplies(ctx context.Context, req *pb.GetTweetRepliesRequest) (*pb.GetTweetRepliesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	users, nextCursor, err := h.xClient.GetTweetReplies(ctx, req.TweetId, req.Cursor, int(req.MaxResults))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "x tweet replies failed: %v", err)
	}

	pbUsers := make([]*pb.TweetUser, 0, len(users))
	for _, user := range users {
		pbUsers = append(pbUsers, &pb.TweetUser{TwitterId: user.TwitterID, Handle: user.Handle})
	}
	return &pb.GetTweetRepliesResponse{Users: pbUsers, NextCursor: nextCursor}, nil
}

func (h *XGatewayGRPCHandler) GetRetweeters(ctx context.Context, req *pb.GetRetweeterRequest) (*pb.GetRetweeterResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	users, nextCursor, err := h.xClient.GetRetweeters(ctx, req.TweetId, req.Cursor, int(req.MaxResults))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "x retweeters failed: %v", err)
	}

	pbUsers := make([]*pb.TweetUser, 0, len(users))
	for _, user := range users {
		pbUsers = append(pbUsers, &pb.TweetUser{TwitterId: user.TwitterID, Handle: user.Handle})
	}
	return &pb.GetRetweeterResponse{Users: pbUsers, NextCursor: nextCursor}, nil
}

func (h *XGatewayGRPCHandler) CheckFollows(ctx context.Context, req *pb.CheckFollowsRequest) (*pb.CheckFollowsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	follows, err := h.xClient.CheckFollows(ctx, req.FollowerId, req.FolloweeId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "x follow check failed: %v", err)
	}
	return &pb.CheckFollowsResponse{Follows: follows}, nil
}

func (h *XGatewayGRPCHandler) GetUserProfile(ctx context.Context, req *pb.GetUserProfileRequest) (*pb.UserProfile, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	profile, err := h.xClient.GetUserProfile(ctx, req.TwitterId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "x profile lookup failed: %v", err)
	}

	return &pb.UserProfile{
		TwitterId:       profile.TwitterID,
		Handle:          profile.Handle,
		FollowerCount:   profile.FollowerCount,
		FollowingCount:  profile.FollowingCount,
		TweetCount:      profile.TweetCount,
		AccountAgeDays:  profile.AccountAgeDays,
		IsVerified:      profile.IsVerified,
		HasProfileImage: profile.HasProfileImage,
	}, nil
}

func (h *XGatewayGRPCHandler) ParseTweetCommand(ctx context.Context, req *pb.ParseTweetCommandRequest) (*pb.GiveawayCommand, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	parser := commandparser.New()
	parsed, err := parser.Parse(req.TweetText, req.SourceTweetId)
	if err != nil {
		return &pb.GiveawayCommand{
			WinnerCount:   1,
			TotalAmount:   100,
			AmountEach:    100,
			Currency:      "NGN",
			EntryRule:     "RANDOM",
			SourceTweetId: req.SourceTweetId,
			Confidence:    0.1,
			RawText:       req.TweetText,
		}, nil
	}

	return &pb.GiveawayCommand{
		WinnerCount:   int32(parsed.WinnerCount),
		TotalAmount:   parsed.TotalAmount,
		AmountEach:    parsed.AmountEach,
		Currency:      string(parsed.Currency),
		EntryRule:     string(parsed.EntryRule),
		SourceTweetId: parsed.SourceTweetID,
		Confidence:    parsed.Confidence,
		RawText:       parsed.RawText,
	}, nil
}
