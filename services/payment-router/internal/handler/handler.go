package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pbGiveaway "github.com/mosesedem/bot-x/gen/go/giveaway/v1"
	pbNotification "github.com/mosesedem/bot-x/gen/go/notification/v1"
	"github.com/mosesedem/bot-x/shared/config"
	"github.com/mosesedem/bot-x/shared/gateways/safehaven"
	"go.uber.org/zap"
)

type WebhookHandler struct {
	db                 *pgxpool.Pool
	giveawayClient     pbGiveaway.GiveawayServiceClient
	notificationClient pbNotification.NotificationServiceClient
	cfg                *config.Config
	logger             *zap.Logger
}

func NewWebhookHandler(
	db *pgxpool.Pool,
	giveawayClient pbGiveaway.GiveawayServiceClient,
	notificationClient pbNotification.NotificationServiceClient,
	cfg *config.Config,
	logger *zap.Logger,
) *WebhookHandler {
	return &WebhookHandler{
		db:                 db,
		giveawayClient:     giveawayClient,
		notificationClient: notificationClient,
		cfg:                cfg,
		logger:             logger,
	}
}

type SafeHavenWebhookPayload struct {
	Event string `json:"event"`
	Data  struct {
		ExternalRef   string  `json:"externalRef"`
		Amount        float64 `json:"amount"`
		Reference     string  `json:"reference"`
		AccountNumber string  `json:"accountNumber"`
		Status        string  `json:"status"`
	} `json:"data"`
}

func (h *WebhookHandler) RegisterRoutes(r chi.Router) {
	r.Post("/webhooks/safehaven", h.HandleSafeHaven)
	r.Post("/webhooks/flutterwave", h.HandleFlutterwave)
	r.Post("/webhooks/paystack", h.HandlePaystack)
}

func (h *WebhookHandler) HandleSafeHaven(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed to read Safe Haven webhook body", zap.Error(err))
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Verify signature if secret is set
	signature := r.Header.Get("X-Safehaven-Signature")
	if h.cfg.SafeHavenClientSecret != "" && signature != "" {
		if !safehaven.VerifyWebhookSignature(body, signature, h.cfg.SafeHavenClientSecret) {
			h.logger.Warn("invalid Safe Haven webhook signature", zap.String("signature", signature))
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	var payload SafeHavenWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		h.logger.Error("failed to unmarshal Safe Haven webhook", zap.Error(err))
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	h.logger.Info("received Safe Haven webhook", zap.String("event", payload.Event))

	ctx := r.Context()
	switch payload.Event {
	case "virtual_account.deposit":
		// Host has funded the escrow
		giveawayID := payload.Data.ExternalRef
		h.logger.Info("escrow funded for giveaway", zap.String("giveaway_id", giveawayID), zap.Float64("amount", payload.Data.Amount))

		// Mark giveaway as active via Giveaway gRPC Service
		_, err := h.giveawayClient.ActivateGiveaway(ctx, &pbGiveaway.GiveawayIDRequest{Id: giveawayID})
		if err != nil {
			h.logger.Error("failed to activate giveaway via gRPC", zap.String("giveaway_id", giveawayID), zap.Error(err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

	case "transfer.success":
		// Outbound payout successful
		ref := payload.Data.Reference
		h.logger.Info("payout transfer successful", zap.String("ref", ref))

		var winnerID, winnerTwitterID, currency, bankName, bankLast4 string
		var amount float64
		err := h.db.QueryRow(ctx, `
			UPDATE giveaway_winners 
			SET payment_status = 'SUCCESS', payout_completed_at = $1 
			WHERE gateway_reference = $2
			RETURNING id, winner_twitter_id, amount, currency, bank_code, payout_destination
		`, time.Now(), ref).Scan(&winnerID, &winnerTwitterID, &amount, &currency, &bankName, &bankLast4)
		
		if err == nil {
			// Trigger payout success DM
			if len(bankLast4) > 4 {
				bankLast4 = bankLast4[len(bankLast4)-4:]
			}
			_, _ = h.notificationClient.SendPayoutSuccessDM(ctx, &pbNotification.PayoutSuccessDMRequest{
				WinnerTwitterId: winnerTwitterID,
				Amount:          amount,
				Currency:        currency,
				BankLast4:       bankLast4,
				BankName:        bankName,
			})
			h.checkAndCompleteGiveaway(ctx, winnerID)
		} else {
			h.logger.Error("failed to update winner payout status to SUCCESS", zap.String("ref", ref), zap.Error(err))
		}

	case "transfer.failed":
		// Outbound payout failed
		ref := payload.Data.Reference
		h.logger.Info("payout transfer failed", zap.String("ref", ref))

		var winnerID, winnerTwitterID, currency string
		var amount float64
		err := h.db.QueryRow(ctx, `
			UPDATE giveaway_winners 
			SET payment_status = 'FAILED' 
			WHERE gateway_reference = $1
			RETURNING id, winner_twitter_id, amount, currency
		`, ref).Scan(&winnerID, &winnerTwitterID, &amount, &currency)

		if err == nil {
			_, _ = h.notificationClient.SendPayoutFailedDM(ctx, &pbNotification.PayoutFailedDMRequest{
				WinnerTwitterId: winnerTwitterID,
				Amount:          amount,
				Currency:        currency,
				Reason:          "Gateway transaction failed",
			})
			h.checkAndCompleteGiveaway(ctx, winnerID)
		} else {
			h.logger.Error("failed to update winner payout status to FAILED", zap.String("ref", ref), zap.Error(err))
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (h *WebhookHandler) HandleFlutterwave(w http.ResponseWriter, r *http.Request) {
	// Stub for Flutterwave
	w.WriteHeader(http.StatusOK)
}

func (h *WebhookHandler) HandlePaystack(w http.ResponseWriter, r *http.Request) {
	// Stub for Paystack
	w.WriteHeader(http.StatusOK)
}

func (h *WebhookHandler) checkAndCompleteGiveaway(ctx context.Context, winnerID string) {
	var giveawayID string
	err := h.db.QueryRow(ctx, "SELECT giveaway_id FROM giveaway_winners WHERE id = $1", winnerID).Scan(&giveawayID)
	if err != nil {
		return
	}

	// Count total winners and those not completed
	var totalWinners, pendingWinners int
	err = h.db.QueryRow(ctx, "SELECT count(*) FROM giveaway_winners WHERE giveaway_id = $1", giveawayID).Scan(&totalWinners)
	if err != nil {
		return
	}
	err = h.db.QueryRow(ctx, "SELECT count(*) FROM giveaway_winners WHERE giveaway_id = $1 AND payment_status IN ('PENDING', 'PROCESSING')", giveawayID).Scan(&pendingWinners)
	if err != nil {
		return
	}

	if pendingWinners == 0 {
		// All payouts completed!
		var paidCount, failedCount int
		var totalDisbursed float64
		var currency, hostTwitterID string

		_ = h.db.QueryRow(ctx, "SELECT count(*) FROM giveaway_winners WHERE giveaway_id = $1 AND payment_status = 'SUCCESS'", giveawayID).Scan(&paidCount)
		_ = h.db.QueryRow(ctx, "SELECT count(*) FROM giveaway_winners WHERE giveaway_id = $1 AND payment_status = 'FAILED'", giveawayID).Scan(&failedCount)
		_ = h.db.QueryRow(ctx, "SELECT COALESCE(sum(amount), 0) FROM giveaway_winners WHERE giveaway_id = $1 AND payment_status = 'SUCCESS'", giveawayID).Scan(&totalDisbursed)
		_ = h.db.QueryRow(ctx, "SELECT host_twitter_id, currency FROM giveaways WHERE id = $1", giveawayID).Scan(&hostTwitterID, &currency)

		// Transition giveaway to COMPLETED
		_, err = h.giveawayClient.UpdateGiveawayStatus(ctx, &pbGiveaway.UpdateStatusRequest{
			Id:     giveawayID,
			Status: "COMPLETED",
			Reason: "All winner payouts processed",
		})

		if err == nil {
			// Send completion DM to the host
			_, _ = h.notificationClient.SendHostCompletionDM(ctx, &pbNotification.HostCompletionDMRequest{
				HostTwitterId:  hostTwitterID,
				GiveawayId:     giveawayID,
				TotalWinners:   int32(totalWinners),
				PaidCount:      int32(paidCount),
				FailedCount:    int32(failedCount),
				TotalDisbursed: totalDisbursed,
				Currency:       currency,
			})
		}
	}
}
