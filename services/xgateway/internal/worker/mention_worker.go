package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	pbAudit "github.com/mosesedem/bot-x/gen/go/audit/v1"
	pbEntry "github.com/mosesedem/bot-x/gen/go/entry/v1"
	pbGiveaway "github.com/mosesedem/bot-x/gen/go/giveaway/v1"
	pbNotification "github.com/mosesedem/bot-x/gen/go/notification/v1"
	pbPayment "github.com/mosesedem/bot-x/gen/go/payment/v1"
	"github.com/mosesedem/bot-x/shared/config"
	"github.com/mosesedem/bot-x/shared/nlp/commandparser"
	"go.uber.org/zap"
)

type EventWorker struct {
	db                 *pgxpool.Pool
	giveawayClient     pbGiveaway.GiveawayServiceClient
	paymentClient      pbPayment.PaymentRouterServiceClient
	notificationClient pbNotification.NotificationServiceClient
	entryClient        pbEntry.EntryServiceClient
	auditClient        pbAudit.AuditServiceClient
	parser             *commandparser.Parser
	logger             *zap.Logger
	cfg                *config.Config
}

func NewEventWorker(
	db *pgxpool.Pool,
	giveawayClient pbGiveaway.GiveawayServiceClient,
	paymentClient pbPayment.PaymentRouterServiceClient,
	notificationClient pbNotification.NotificationServiceClient,
	entryClient pbEntry.EntryServiceClient,
	auditClient pbAudit.AuditServiceClient,
	logger *zap.Logger,
	cfg *config.Config,
) *EventWorker {
	return &EventWorker{
		db:                 db,
		giveawayClient:     giveawayClient,
		paymentClient:      paymentClient,
		notificationClient: notificationClient,
		entryClient:        entryClient,
		auditClient:        auditClient,
		parser:             commandparser.New(),
		logger:             logger,
		cfg:                cfg,
	}
}

type XWebhookEventWrapper struct {
	TweetCreateEvents []struct {
		IDStr string `json:"id_str"`
		Text  string `json:"text"`
		User  struct {
			IDStr      string `json:"id_str"`
			ScreenName string `json:"screen_name"`
		} `json:"user"`
	} `json:"tweet_create_events"`
	DirectMessageEvents []struct {
		ID      string `json:"id"`
		Message struct {
			SenderID string `json:"sender_id"`
			Text     string `json:"text"`
		} `json:"message_create"`
	} `json:"direct_message_events"`
}

func (w *EventWorker) ProcessTask(ctx context.Context, t *asynq.Task) error {
	w.logger.Info("processing X webhook event from task queue")

	var wrapper XWebhookEventWrapper
	if err := json.Unmarshal(t.Payload(), &wrapper); err != nil {
		return fmt.Errorf("failed to unmarshal task payload: %w", err)
	}

	// 1. Process Tweet Mention Commands
	for _, tweet := range wrapper.TweetCreateEvents {
		if tweet.User.IDStr == w.cfg.BotTwitterID {
			continue // skip events from bot itself
		}
		w.logger.Info("processing tweet mention", zap.String("tweet_id", tweet.IDStr), zap.String("author", tweet.User.ScreenName))
		if err := w.handleTweetMention(ctx, tweet.IDStr, tweet.Text, tweet.User.IDStr, tweet.User.ScreenName); err != nil {
			w.logger.Error("failed to handle tweet mention", zap.Error(err))
		}
	}

	// 2. Process DMs
	for _, dm := range wrapper.DirectMessageEvents {
		if dm.Message.SenderID == w.cfg.BotTwitterID {
			continue // skip bot's own DMs
		}
		w.logger.Info("processing DM", zap.String("dm_id", dm.ID), zap.String("sender", dm.Message.SenderID))
		if err := w.handleDM(ctx, dm.Message.SenderID, dm.Message.Text); err != nil {
			w.logger.Error("failed to handle DM", zap.Error(err))
		}
	}

	return nil
}

func (w *EventWorker) handleTweetMention(ctx context.Context, tweetID, text, authorID, handle string) error {
	// Register host profile if not exists
	_, err := w.db.Exec(ctx, `
		INSERT INTO host_profiles (twitter_id, twitter_handle)
		VALUES ($1, $2) ON CONFLICT (twitter_id) DO NOTHING
	`, authorID, handle)
	if err != nil {
		w.logger.Warn("failed to save host profile", zap.Error(err))
	}

	// NLP Parse Command
	cmd, err := w.parser.Parse(text, tweetID)
	if err != nil {
		w.logger.Warn("nlp command parsing failed", zap.String("text", text), zap.Error(err))
		return nil // don't retry, it's just not a valid command
	}

	// Call Giveaway Service gRPC to create draft
	g, err := w.giveawayClient.CreateGiveaway(ctx, &pbGiveaway.CreateGiveawayRequest{
		HostTwitterId:   authorID,
		SourceTweetId:   tweetID,
		CommandTweetId:  tweetID,
		TotalBudget:     cmd.TotalAmount,
		Currency:        string(cmd.Currency),
		WinnerCount:     int32(cmd.WinnerCount),
		AmountPerWinner: cmd.AmountEach,
		EntryRule:       string(cmd.EntryRule),
		Jurisdiction:    "NG", // default NG
	})
	if err != nil {
		return fmt.Errorf("failed to create draft giveaway: %w", err)
	}

	// Call Payment Router to initiate escrow
	escrow, err := w.paymentClient.InitiateEscrow(ctx, &pbPayment.InitiateEscrowRequest{
		GiveawayId:    g.Id,
		Amount:        g.TotalBudget,
		Currency:      g.Currency,
		Jurisdiction:  g.Jurisdiction,
		HostTwitterId: g.HostTwitterId,
	})
	if err != nil {
		return fmt.Errorf("failed to initiate escrow for draft giveaway: %w", err)
	}

	// Compute 2% platform fee in smallest denomination and DM host
	fee := (g.TotalBudget*2 + 50) / 100 // 2% fee, rounded half-up
	totalToFund := g.TotalBudget + fee
	_, err = w.notificationClient.SendGiveawayConfirmationDM(ctx, &pbNotification.GiveawayConfirmationDMRequest{
		HostTwitterId:        g.HostTwitterId,
		GiveawayId:           g.Id,
		WinnerCount:          g.WinnerCount,
		AmountPerWinner:      g.AmountPerWinner,
		Currency:             g.Currency,
		EntryRule:            g.EntryRule,
		VirtualAccountNumber: escrow.VirtualAccountNumber,
		BankName:             escrow.BankName,
		AccountName:          escrow.AccountName,
		TotalToFund:          totalToFund,
	})
	if err != nil {
		return fmt.Errorf("failed to DM host: %w", err)
	}

	return nil
}

func (w *EventWorker) handleDM(ctx context.Context, senderID, text string) error {
	textLower := strings.ToLower(strings.TrimSpace(text))

	// Check if this is a host responding to YES/NO confirmation of a DRAFT giveaway
	var draftID string
	var totalBudgetInt int64
	var currency string
	err := w.db.QueryRow(ctx, `
		SELECT id, total_budget, currency 
		FROM giveaways 
		WHERE host_twitter_id = $1 AND status = 'DRAFT' 
		ORDER BY created_at DESC LIMIT 1
	`, senderID).Scan(&draftID, &totalBudgetInt, &currency)

	if err == nil {
		if textLower == "yes" {
			// Acknowledge YES, remind host to make the transfer
			w.logger.Info("host confirmed giveaway via DM", zap.String("giveaway_id", draftID))
			return nil // Escrow inflow webhook will activate the giveaway when funds arrive
		} else if textLower == "no" {
			w.logger.Info("host cancelled giveaway via DM", zap.String("giveaway_id", draftID))
			_, err = w.giveawayClient.CancelGiveaway(ctx, &pbGiveaway.CancelRequest{
				Id:     draftID,
				Reason: "Cancelled by host request via DM",
			})
			return err
		}
	}

	// Check if this is a winner replying with bank details
	bankName, accountNum, accountName := w.parseBankDetails(text)
	if accountNum != "" {
		w.logger.Info("parsed bank details from winner", zap.String("winner_id", senderID), zap.String("acc", accountNum))

		// Find the latest PENDING winner record for this X ID
		var winID, giveawayID, winCurrency string
		var winAmountInt int64
		err = w.db.QueryRow(ctx, `
			SELECT id, giveaway_id, amount, currency 
			FROM giveaway_winners 
			WHERE winner_twitter_id = $1 AND payment_status IN ('PENDING', 'FAILED') 
			ORDER BY created_at DESC LIMIT 1
		`, senderID).Scan(&winID, &giveawayID, &winAmountInt, &winCurrency)

		if err == nil {
			// Map Bank Name to Bank Code (e.g. Wema -> 035, GTB -> 058, Zenith -> 057)
			bankCode := w.mapBankCode(bankName)

			// Update winner record in DB
			_, err = w.db.Exec(ctx, `
				UPDATE giveaway_winners 
				SET payout_destination = $1, payout_destination_type = 'bank', bank_code = $2, winner_twitter_handle = $3 
				WHERE id = $4
			`, accountNum, bankCode, bankName, winID)
			if err != nil {
				return fmt.Errorf("failed to save winner bank details: %w", err)
			}

			// Retrieve host jurisdiction
			var jur string
			_ = w.db.QueryRow(ctx, "SELECT jurisdiction FROM giveaways WHERE id = $1", giveawayID).Scan(&jur)
			if jur == "" {
				jur = "NG"
			}

			// Dispatch Payment (send stored cents as int64)
			_, err = w.paymentClient.RoutePayment(ctx, &pbPayment.RoutePaymentRequest{
				WinnerId:              winID,
				GiveawayId:            giveawayID,
				TwitterId:             senderID,
				Amount:                winAmountInt,
				Currency:              winCurrency,
				Jurisdiction:          jur,
				PayoutDestination:     accountNum,
				PayoutDestinationType: "bank",
				BankCode:              bankCode,
				BeneficiaryName:       accountName,
				IdempotencyKey:        winID, // use winner record UUID as idempotency key
			})
			return err
		}
	}

	return nil
}

func (w *EventWorker) parseBankDetails(text string) (bankName, accountNum, accountName string) {
	lines := strings.Split(text, "\n")
	for _, l := range lines {
		lLower := strings.ToLower(l)
		if strings.Contains(lLower, "bank name") {
			parts := strings.Split(l, ":")
			if len(parts) > 1 {
				bankName = strings.TrimSpace(parts[1])
			}
		} else if strings.Contains(lLower, "account number") {
			parts := strings.Split(l, ":")
			if len(parts) > 1 {
				accountNum = strings.TrimSpace(parts[1])
			}
		} else if strings.Contains(lLower, "account name") {
			parts := strings.Split(l, ":")
			if len(parts) > 1 {
				accountName = strings.TrimSpace(parts[1])
			}
		}
	}
	reg := regexp.MustCompile(`\D`)
	accountNum = reg.ReplaceAllString(accountNum, "")
	return
}

func (w *EventWorker) mapBankCode(bankName string) string {
	bn := strings.ToLower(bankName)
	if strings.Contains(bn, "wema") {
		return "035"
	}
	if strings.Contains(bn, "gt") || strings.Contains(bn, "guaranty") {
		return "058"
	}
	if strings.Contains(bn, "zenith") {
		return "057"
	}
	if strings.Contains(bn, "access") {
		return "044"
	}
	if strings.Contains(bn, "uba") || strings.Contains(bn, "united bank") {
		return "033"
	}
	if strings.Contains(bn, "first") {
		return "011"
	}
	if strings.Contains(bn, "union") {
		return "032"
	}
	if strings.Contains(bn, "polaris") {
		return "076"
	}
	if strings.Contains(bn, "fidelity") {
		return "070"
	}
	if strings.Contains(bn, "stanbic") {
		return "221"
	}
	if strings.Contains(bn, "sterling") {
		return "232"
	}
	return "011" // default to First Bank
}
