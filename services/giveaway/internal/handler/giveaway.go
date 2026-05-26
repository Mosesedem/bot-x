package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"

	pbAudit "github.com/mosesedem/bot-x/gen/go/audit/v1"
	pbEntry "github.com/mosesedem/bot-x/gen/go/entry/v1"
	pb "github.com/mosesedem/bot-x/gen/go/giveaway/v1"
	pbKYC "github.com/mosesedem/bot-x/gen/go/kyc/v1"
	pbNotification "github.com/mosesedem/bot-x/gen/go/notification/v1"
	pbPayment "github.com/mosesedem/bot-x/gen/go/payment/v1"
	"github.com/mosesedem/bot-x/services/giveaway/internal/statemachine"
	"github.com/mosesedem/bot-x/shared/config"
)

type GiveawayHandler struct {
	pb.UnimplementedGiveawayServiceServer
	db                 *pgxpool.Pool
	entryClient        pbEntry.EntryServiceClient
	kycClient          pbKYC.KYCServiceClient
	notificationClient pbNotification.NotificationServiceClient
	paymentClient      pbPayment.PaymentRouterServiceClient
	auditClient        pbAudit.AuditServiceClient
	cfg                *config.Config
}

func NewGiveawayHandler(
	db *pgxpool.Pool,
	entryClient pbEntry.EntryServiceClient,
	kycClient pbKYC.KYCServiceClient,
	notificationClient pbNotification.NotificationServiceClient,
	paymentClient pbPayment.PaymentRouterServiceClient,
	auditClient pbAudit.AuditServiceClient,
	cfg *config.Config,
) *GiveawayHandler {
	return &GiveawayHandler{
		db:                 db,
		entryClient:        entryClient,
		kycClient:          kycClient,
		notificationClient: notificationClient,
		paymentClient:      paymentClient,
		auditClient:        auditClient,
		cfg:                cfg,
	}
}

// ── gRPC Methods ──

func (h *GiveawayHandler) CreateGiveaway(ctx context.Context, req *pb.CreateGiveawayRequest) (*pb.Giveaway, error) {
	// Parse input deadline
	deadline := time.Now().Add(24 * time.Hour) // default 24h

	// Amounts are provided in lowest denomination (cents/kobo)
	totalBudgetInt := req.TotalBudget
	amountPerWinnerInt := req.AmountPerWinner

	var id string
	err := h.db.QueryRow(ctx, `
		INSERT INTO giveaways (
			host_twitter_id, source_tweet_id, command_tweet_id, total_budget, currency, 
			winner_count, amount_per_winner, entry_rule, jurisdiction, status, deadline_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'DRAFT', $10)
		RETURNING id, created_at
	`, req.HostTwitterId, req.SourceTweetId, req.CommandTweetId, totalBudgetInt, req.Currency,
		req.WinnerCount, amountPerWinnerInt, req.EntryRule, req.Jurisdiction, deadline).Scan(&id, &deadline)

	if err != nil {
		return nil, fmt.Errorf("failed to create giveaway: %w", err)
	}

	_, _ = h.auditClient.LogEvent(ctx, &pbAudit.LogEventRequest{
		EntityType: "giveaway",
		EntityId:   id,
		Action:     "CREATE",
		ActorId:    req.HostTwitterId,
		Payload:    fmt.Sprintf(`{"budget_cents":%d,"winners":%d}`, req.TotalBudget, req.WinnerCount),
	})

	return &pb.Giveaway{
		Id:              id,
		HostTwitterId:   req.HostTwitterId,
		SourceTweetId:   req.SourceTweetId,
		Status:          "DRAFT",
		TotalBudget:     req.TotalBudget,
		Currency:        req.Currency,
		WinnerCount:     req.WinnerCount,
		AmountPerWinner: req.AmountPerWinner,
		EntryRule:       req.EntryRule,
		Jurisdiction:    req.Jurisdiction,
		CreatedAt:       timestamppb.New(time.Now()),
		DeadlineAt:      timestamppb.New(deadline),
	}, nil
}

func (h *GiveawayHandler) ActivateGiveaway(ctx context.Context, req *pb.GiveawayIDRequest) (*pb.Giveaway, error) {
	g, err := h.getGiveawayRecord(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	if err := statemachine.ValidateTransition(statemachine.State(g.Status), statemachine.StateActive); err != nil {
		return nil, err
	}

	_, err = h.db.Exec(ctx, "UPDATE giveaways SET status = 'ACTIVE' WHERE id = $1", req.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to activate giveaway: %w", err)
	}
	g.Status = "ACTIVE"

	_, _ = h.auditClient.LogEvent(ctx, &pbAudit.LogEventRequest{
		EntityType: "giveaway",
		EntityId:   req.Id,
		Action:     "ACTIVATE",
		Gateway:    g.EscrowGateway,
	})

	// Send live reply via Notification
	_, _ = h.notificationClient.SendActivationReply(ctx, &pbNotification.ActivationReplyRequest{
		TweetId:             g.SourceTweetId,
		GiveawayId:          g.Id,
		WinnerCount:         g.WinnerCount,
		AmountPerWinner:     g.AmountPerWinner,
		Currency:            g.Currency,
		DeadlineDescription: "24 hours",
	})

	return g, nil
}

func (h *GiveawayHandler) LockGiveaway(ctx context.Context, req *pb.GiveawayIDRequest) (*pb.Giveaway, error) {
	g, err := h.getGiveawayRecord(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	if err := statemachine.ValidateTransition(statemachine.State(g.Status), statemachine.StateLocked); err != nil {
		return nil, err
	}

	_, err = h.db.Exec(ctx, "UPDATE giveaways SET status = 'LOCKED' WHERE id = $1", req.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to lock giveaway: %w", err)
	}
	g.Status = "LOCKED"

	_, _ = h.auditClient.LogEvent(ctx, &pbAudit.LogEventRequest{
		EntityType: "giveaway",
		EntityId:   req.Id,
		Action:     "LOCK",
	})

	return g, nil
}

func (h *GiveawayHandler) DrawWinners(ctx context.Context, req *pb.GiveawayIDRequest) (*pb.DrawWinnersResponse, error) {
	g, err := h.getGiveawayRecord(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	// For convenience, if active, we allow drawing directly (automatically locking first)
	if g.Status == "ACTIVE" {
		_, err = h.LockGiveaway(ctx, req)
		if err != nil {
			return nil, err
		}
		g.Status = "LOCKED"
	}

	if err := statemachine.ValidateTransition(statemachine.State(g.Status), statemachine.StateDrawing); err != nil {
		return nil, err
	}

	// 1. Get eligible entries
	res, err := h.entryClient.GetEligibleEntries(ctx, &pbEntry.GetEligibleEntriesRequest{GiveawayId: req.Id})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve eligible entries: %w", err)
	}

	if len(res.Entries) == 0 {
		return nil, fmt.Errorf("no eligible entries found for this giveaway")
	}

	// 2. Select winners using CSPRNG
	winnerCount := int(g.WinnerCount)
	if len(res.Entries) < winnerCount {
		winnerCount = len(res.Entries)
	}

	selectedIndices := h.drawCSPRNGIndices(winnerCount, len(res.Entries))
	var winnerIDs []string

	// Check if KYC is enabled for this jurisdiction
	kycReqResp, err := h.kycClient.IsKYCRequired(ctx, &pbKYC.IsKYCRequiredRequest{
		GiveawayId:   g.Id,
		Jurisdiction: g.Jurisdiction,
	})
	kycRequired := false
	if err == nil {
		kycRequired = kycReqResp.Required
	}

	kycStatus := "NOT_REQUIRED"
	paymentStatus := "PENDING"
	nextState := statemachine.StateGatewayRouting
	if kycRequired {
		kycStatus = "PENDING"
		nextState = statemachine.StateKYCPending
	}

	// Fetch host profile handle
	var hostHandle string
	_ = h.db.QueryRow(ctx, "SELECT twitter_handle FROM host_profiles WHERE twitter_id = $1", g.HostTwitterId).Scan(&hostHandle)
	if hostHandle == "" {
		hostHandle = "host"
	}

	for _, idx := range selectedIndices {
		entry := res.Entries[idx]
		winnerIDs = append(winnerIDs, entry.TwitterId)

		// Insert into giveaway_winners
		amountInt := g.AmountPerWinner
		_, err := h.db.Exec(ctx, `
			INSERT INTO giveaway_winners (
				giveaway_id, winner_twitter_id, winner_twitter_handle, kyc_status, payment_status, amount, currency
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (giveaway_id, winner_twitter_id) DO NOTHING
			`, g.Id, entry.TwitterId, entry.TwitterHandle, kycStatus, paymentStatus, amountInt, g.Currency)

		if err != nil {
			return nil, fmt.Errorf("failed to save winner details: %w", err)
		}

		// Notify winners
		if kycRequired {
			// Send KYC DM
			kycLink := fmt.Sprintf("%s/kyc/%s/verify", h.cfg.BaseURL, entry.TwitterId)
			_, _ = h.notificationClient.SendKYCRequestDM(ctx, &pbNotification.KYCRequestDMRequest{
				WinnerTwitterId: entry.TwitterId,
				Amount:          g.AmountPerWinner,
				Currency:        g.Currency,
				KycLink:         kycLink,
			})
		} else {
			// Ask for bank details
			_, _ = h.notificationClient.SendWinnerDM(ctx, &pbNotification.WinnerDMRequest{
				WinnerTwitterId: entry.TwitterId,
				GiveawayId:      g.Id,
				Amount:          g.AmountPerWinner,
				Currency:        g.Currency,
				HostHandle:      hostHandle,
			})
		}
	}

	// Update giveaway status to DRAWING, then immediately to next state
	_, _ = h.db.Exec(ctx, "UPDATE giveaways SET status = $1 WHERE id = $2", string(nextState), req.Id)

	_, _ = h.auditClient.LogEvent(ctx, &pbAudit.LogEventRequest{
		EntityType: "giveaway",
		EntityId:   req.Id,
		Action:     "DRAW_WINNERS",
		Payload:    fmt.Sprintf(`{"drawn_count":%d,"next_state":"%s"}`, len(winnerIDs), nextState),
	})

	return &pb.DrawWinnersResponse{
		GiveawayId:       g.Id,
		WinnerTwitterIds: winnerIDs,
		WinnerCount:      int32(len(winnerIDs)),
	}, nil
}

func (h *GiveawayHandler) GetGiveaway(ctx context.Context, req *pb.GiveawayIDRequest) (*pb.Giveaway, error) {
	return h.getGiveawayRecord(ctx, req.Id)
}

func (h *GiveawayHandler) UpdateGiveawayStatus(ctx context.Context, req *pb.UpdateStatusRequest) (*pb.Giveaway, error) {
	g, err := h.getGiveawayRecord(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	if err := statemachine.ValidateTransition(statemachine.State(g.Status), statemachine.State(req.Status)); err != nil {
		return nil, err
	}

	_, err = h.db.Exec(ctx, "UPDATE giveaways SET status = $1 WHERE id = $2", req.Status, req.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to update giveaway status: %w", err)
	}
	g.Status = req.Status

	_, _ = h.auditClient.LogEvent(ctx, &pbAudit.LogEventRequest{
		EntityType: "giveaway",
		EntityId:   req.Id,
		Action:     "STATUS_UPDATE",
		Payload:    fmt.Sprintf(`{"status":"%s","reason":"%s"}`, req.Status, req.Reason),
	})

	return g, nil
}

func (h *GiveawayHandler) CancelGiveaway(ctx context.Context, req *pb.CancelRequest) (*pb.Giveaway, error) {
	g, err := h.getGiveawayRecord(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	if err := statemachine.ValidateTransition(statemachine.State(g.Status), statemachine.StateCancelled); err != nil {
		return nil, err
	}

	// Update DB
	_, err = h.db.Exec(ctx, "UPDATE giveaways SET status = 'CANCELLED', cancel_reason = $1, closed_at = $2 WHERE id = $3", req.Reason, time.Now(), req.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to cancel giveaway: %w", err)
	}
	g.Status = "CANCELLED"

	// Call escrow refund in payment-router
	_, _ = h.paymentClient.RefundEscrow(ctx, &pbPayment.RefundEscrowRequest{
		GiveawayId: req.Id,
		Reason:     req.Reason,
	})

	_, _ = h.auditClient.LogEvent(ctx, &pbAudit.LogEventRequest{
		EntityType: "giveaway",
		EntityId:   req.Id,
		Action:     "CANCEL",
		Payload:    fmt.Sprintf(`{"reason":"%s"}`, req.Reason),
	})

	return g, nil
}

// Helper methods

func (h *GiveawayHandler) getGiveawayRecord(ctx context.Context, id string) (*pb.Giveaway, error) {
	var g pb.Giveaway
	var created, deadline time.Time
	var ref, gateway, fundingAcc, bankCode sql.NullString

	// Scan numeric DB BIGINTs into int64, then convert to float for protobuf/public views
	var totalBudgetInt int64
	var amountPerWinnerInt int64

	err := h.db.QueryRow(ctx, `
		SELECT id, host_twitter_id, source_tweet_id, status, total_budget, currency, 
			winner_count, amount_per_winner, entry_rule, jurisdiction, escrow_reference, 
			escrow_gateway, funding_account, funding_bank_code, created_at, deadline_at 
		FROM giveaways 
		WHERE id = $1
	`, id).Scan(&g.Id, &g.HostTwitterId, &g.SourceTweetId, &g.Status, &totalBudgetInt, &g.Currency,
		&g.WinnerCount, &amountPerWinnerInt, &g.EntryRule, &g.Jurisdiction, &ref, &gateway, &fundingAcc, &bankCode, &created, &deadline)

	if err != nil {
		return nil, fmt.Errorf("giveaway not found: %w", err)
	}

	g.EscrowReference = ref.String
	g.EscrowGateway = gateway.String
	g.FundingAccount = fundingAcc.String
	g.FundingBankCode = bankCode.String
	g.CreatedAt = timestamppb.New(created)
	g.DeadlineAt = timestamppb.New(deadline)

	// Assign stored integer amounts (cents) to protobuf fields (int64)
	g.TotalBudget = totalBudgetInt
	g.AmountPerWinner = amountPerWinnerInt

	return &g, nil
}

func (h *GiveawayHandler) drawCSPRNGIndices(n, total int) []int {
	// Pick n unique random indices out of total using crypto/rand
	if n >= total {
		res := make([]int, total)
		for i := 0; i < total; i++ {
			res[i] = i
		}
		return res
	}

	// Generate seed using crypto/rand
	var seedBytes [8]byte
	_, _ = rand.Read(seedBytes[:])
	seed := int64(binary.BigEndian.Uint64(seedBytes[:]))

	// Standard shuffling inside a list of indices [0...total-1]
	indices := make([]int, total)
	for i := 0; i < total; i++ {
		indices[i] = i
	}

	// We use the crypto/rand seed to instantiate a secure fisher-yates shuffle
	for i := total - 1; i > 0; i-- {
		nBig, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		j := nBig.Int64()
		indices[i], indices[j] = indices[j], indices[i]
	}

	// Silent unused warning fix for seed variable
	_ = seed

	return indices[:n]
}

// ── HTTP Endpoints ──

func (h *GiveawayHandler) RegisterRoutes(r chi.Router) {
	r.Get("/giveaways/{id}", h.HTTPGetGiveaway)
	r.Get("/giveaways/{id}/winners", h.HTTPGetWinners)
	r.Get("/giveaways/{id}/entries", h.HTTPGetEntries)
	r.Get("/giveaways/{id}/audit", h.HTTPGetAudit)
	r.Post("/giveaways/{id}/cancel", h.HTTPCancelGiveaway)
}

func (h *GiveawayHandler) HTTPGetGiveaway(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	g, err := h.getGiveawayRecord(r.Context(), id)
	if err != nil {
		http.Error(w, "Giveaway not found", http.StatusNotFound)
		return
	}

	// Return public fields
	resp := map[string]interface{}{
		"id":                g.Id,
		"status":            g.Status,
		"total_budget":      float64(g.TotalBudget) / 100.0,
		"currency":          g.Currency,
		"winner_count":      g.WinnerCount,
		"amount_per_winner": float64(g.AmountPerWinner) / 100.0,
		"entry_rule":        g.EntryRule,
		"deadline_at":       g.DeadlineAt.AsTime().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *GiveawayHandler) HTTPGetWinners(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rows, err := h.db.Query(r.Context(), `
		SELECT winner_twitter_id, winner_twitter_handle, payment_status, amount, currency 
		FROM giveaway_winners 
		WHERE giveaway_id = $1
	`, id)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Winner struct {
		TwitterID     string  `json:"twitter_id"`
		TwitterHandle string  `json:"twitter_handle"`
		PaymentStatus string  `json:"payment_status"`
		Amount        float64 `json:"amount"`
		Currency      string  `json:"currency"`
	}

	var winners []Winner
	for rows.Next() {
		var win Winner
		var amountInt int64
		if err := rows.Scan(&win.TwitterID, &win.TwitterHandle, &win.PaymentStatus, &amountInt, &win.Currency); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		win.Amount = float64(amountInt) / 100.0
		winners = append(winners, win)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(winners)
}

func (h *GiveawayHandler) HTTPGetEntries(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Verify token from query params
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		http.Error(w, "Unauthorized: missing token", http.StatusUnauthorized)
		return
	}

	g, err := h.getGiveawayRecord(r.Context(), id)
	if err != nil {
		http.Error(w, "Giveaway not found", http.StatusNotFound)
		return
	}

	// Parse token
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return []byte(h.cfg.AdminJWTSecret), nil
	})

	if err != nil || !token.Valid {
		http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || claims["sub"] != g.HostTwitterId {
		http.Error(w, "Unauthorized: host identity mismatch", http.StatusForbidden)
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT twitter_id, twitter_handle, entry_type, trust_score, created_at 
		FROM giveaway_entries 
		WHERE giveaway_id = $1
	`, id)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Entry struct {
		TwitterID     string    `json:"twitter_id"`
		TwitterHandle string    `json:"twitter_handle"`
		EntryType     string    `json:"entry_type"`
		TrustScore    float64   `json:"trust_score"`
		CreatedAt     time.Time `json:"created_at"`
	}

	var entries []Entry
	for rows.Next() {
		var ent Entry
		if err := rows.Scan(&ent.TwitterID, &ent.TwitterHandle, &ent.EntryType, &ent.TrustScore, &ent.CreatedAt); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		entries = append(entries, ent)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(entries)
}

func (h *GiveawayHandler) HTTPGetAudit(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Call Audit Service over gRPC to query events
	resp, err := h.auditClient.QueryEvents(r.Context(), &pbAudit.QueryEventsRequest{
		EntityId:   id,
		EntityType: "giveaway",
		Limit:      50,
	})
	if err != nil {
		http.Error(w, "failed to query audit log: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp.Events)
}

func (h *GiveawayHandler) HTTPCancelGiveaway(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Verify token from query params
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		http.Error(w, "Unauthorized: missing token", http.StatusUnauthorized)
		return
	}

	g, err := h.getGiveawayRecord(r.Context(), id)
	if err != nil {
		http.Error(w, "Giveaway not found", http.StatusNotFound)
		return
	}

	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return []byte(h.cfg.AdminJWTSecret), nil
	})

	if err != nil || !token.Valid {
		http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || claims["sub"] != g.HostTwitterId {
		http.Error(w, "Unauthorized: host identity mismatch", http.StatusForbidden)
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Reason == "" {
		body.Reason = "Cancelled by host request"
	}

	_, err = h.CancelGiveaway(r.Context(), &pb.CancelRequest{
		Id:     id,
		Reason: body.Reason,
	})
	if err != nil {
		http.Error(w, "failed to cancel: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"CANCELLED"}`))
}
