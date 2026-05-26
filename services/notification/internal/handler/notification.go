package handler

import (
	"context"
	"fmt"

	pb "github.com/mosesedem/bot-x/gen/go/notification/v1"
	pbX "github.com/mosesedem/bot-x/gen/go/xgateway/v1"
)

type NotificationHandler struct {
	pb.UnimplementedNotificationServiceServer
	xGatewayClient pbX.XGatewayServiceClient
}

func NewNotificationHandler(xGatewayClient pbX.XGatewayServiceClient) *NotificationHandler {
	return &NotificationHandler{
		xGatewayClient: xGatewayClient,
	}
}

func (h *NotificationHandler) SendGiveawayConfirmationDM(ctx context.Context, req *pb.GiveawayConfirmationDMRequest) (*pb.NotificationResponse, error) {
	msg := fmt.Sprintf(
		"Hi 👋 I can run this giveaway for you!\n\n"+
			"Prize pool: %s%.2f (%d winners × %s%.2f)\n"+
			"Rule: %s\n\n"+
			"To activate, fund the prize pool + our 2%% fee (%s%.2f total):\n"+
			"Bank: %s\n"+
			"Account Number: %s\n"+
			"Account Name: %s\n\n"+
			"Reply YES to confirm or NO to cancel. Pool expires in 2 hours.",
		req.Currency, float64(req.AmountPerWinner*int64(req.WinnerCount))/100.0,
		req.WinnerCount, req.Currency, float64(req.AmountPerWinner)/100.0,
		req.EntryRule,
		req.Currency, float64(req.TotalToFund)/100.0,
		req.BankName, req.VirtualAccountNumber, req.AccountName,
	)

	resp, err := h.xGatewayClient.SendDM(ctx, &pbX.SendDMRequest{
		UserId:  req.HostTwitterId,
		Message: msg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send DM via X Gateway: %w", err)
	}

	return &pb.NotificationResponse{
		Sent:      true,
		MessageId: resp.DmEventId,
	}, nil
}

func (h *NotificationHandler) SendActivationReply(ctx context.Context, req *pb.ActivationReplyRequest) (*pb.NotificationResponse, error) {
	msg := fmt.Sprintf(
		"🎉 Giveaway is LIVE! Reply to enter.\n"+
			"Prize pool: %s%.2f (%d winners × %s%.2f).\n"+
			"Closes: %s.",
		req.Currency, float64(req.AmountPerWinner*int64(req.WinnerCount))/100.0,
		req.WinnerCount, req.Currency, float64(req.AmountPerWinner)/100.0,
		req.DeadlineDescription,
	)

	resp, err := h.xGatewayClient.ReplyToTweet(ctx, &pbX.ReplyToTweetRequest{
		TweetId: req.TweetId,
		Text:    msg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send tweet reply via X Gateway: %w", err)
	}

	return &pb.NotificationResponse{
		Sent:      true,
		MessageId: resp.ReplyTweetId,
	}, nil
}

func (h *NotificationHandler) SendWinnerDM(ctx context.Context, req *pb.WinnerDMRequest) (*pb.NotificationResponse, error) {
	msg := fmt.Sprintf(
		"🏆 You won %s%.2f in @%s's giveaway!\n\n"+
			"Please reply with your bank details in this format:\n"+
			"Bank Name: <Your Bank Name>\n"+
			"Account Number: <Your Account Number>\n"+
			"Account Name: <Your Account Name>",
		req.Currency, float64(req.Amount)/100.0, req.HostHandle,
	)

	resp, err := h.xGatewayClient.SendDM(ctx, &pbX.SendDMRequest{
		UserId:  req.WinnerTwitterId,
		Message: msg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send winner DM via X Gateway: %w", err)
	}

	return &pb.NotificationResponse{
		Sent:      true,
		MessageId: resp.DmEventId,
	}, nil
}

func (h *NotificationHandler) SendPayoutSuccessDM(ctx context.Context, req *pb.PayoutSuccessDMRequest) (*pb.NotificationResponse, error) {
	msg := fmt.Sprintf(
		"✅ %s%.2f has been successfully paid to your %s account ending in %s.",
		req.Currency, float64(req.Amount)/100.0, req.BankName, req.BankLast4,
	)

	resp, err := h.xGatewayClient.SendDM(ctx, &pbX.SendDMRequest{
		UserId:  req.WinnerTwitterId,
		Message: msg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send payout success DM via X Gateway: %w", err)
	}

	return &pb.NotificationResponse{
		Sent:      true,
		MessageId: resp.DmEventId,
	}, nil
}

func (h *NotificationHandler) SendPayoutFailedDM(ctx context.Context, req *pb.PayoutFailedDMRequest) (*pb.NotificationResponse, error) {
	msg := fmt.Sprintf(
		"❌ Payout of %s%.2f failed.\n"+
			"Reason: %s\n\n"+
			"Please reply with your corrected bank details in this format:\n"+
			"Bank Name: <Correct Bank Name>\n"+
			"Account Number: <Correct Account Number>\n"+
			"Account Name: <Correct Account Name>",
		req.Currency, float64(req.Amount)/100.0, req.Reason,
	)

	resp, err := h.xGatewayClient.SendDM(ctx, &pbX.SendDMRequest{
		UserId:  req.WinnerTwitterId,
		Message: msg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send payout failed DM via X Gateway: %w", err)
	}

	return &pb.NotificationResponse{
		Sent:      true,
		MessageId: resp.DmEventId,
	}, nil
}

func (h *NotificationHandler) SendHostCompletionDM(ctx context.Context, req *pb.HostCompletionDMRequest) (*pb.NotificationResponse, error) {
	msg := fmt.Sprintf(
		"🎉 Giveaway complete!\n\n"+
			"Summary for Giveaway %s:\n"+
			"- Total winners: %d\n"+
			"- Paid successfully: %d\n"+
			"- Failed payouts: %d\n"+
			"- Total disbursed: %s%.2f",
		req.GiveawayId, req.TotalWinners, req.PaidCount, req.FailedCount, req.Currency, float64(req.TotalDisbursed)/100.0,
	)

	resp, err := h.xGatewayClient.SendDM(ctx, &pbX.SendDMRequest{
		UserId:  req.HostTwitterId,
		Message: msg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send host completion DM via X Gateway: %w", err)
	}

	return &pb.NotificationResponse{
		Sent:      true,
		MessageId: resp.DmEventId,
	}, nil
}

func (h *NotificationHandler) SendKYCRequestDM(ctx context.Context, req *pb.KYCRequestDMRequest) (*pb.NotificationResponse, error) {
	msg := fmt.Sprintf(
		"⚠️ Verification required!\n\n"+
			"To claim your prize of %s%.2f, please complete identity verification using the link below:\n"+
			"%s",
		req.Currency, float64(req.Amount)/100.0, req.KycLink,
	)

	resp, err := h.xGatewayClient.SendDM(ctx, &pbX.SendDMRequest{
		UserId:  req.WinnerTwitterId,
		Message: msg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send KYC request DM via X Gateway: %w", err)
	}

	return &pb.NotificationResponse{
		Sent:      true,
		MessageId: resp.DmEventId,
	}, nil
}
