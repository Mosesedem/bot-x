package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	pbAudit "github.com/mosesedem/bot-x/gen/go/audit/v1"
	pbCompliance "github.com/mosesedem/bot-x/gen/go/compliance/v1"
	pbGiveaway "github.com/mosesedem/bot-x/gen/go/giveaway/v1"
	pbKYC "github.com/mosesedem/bot-x/gen/go/kyc/v1"
	pbPayment "github.com/mosesedem/bot-x/gen/go/payment/v1"
	"github.com/mosesedem/bot-x/shared/config"
)

type AdminHandler struct {
	giveawayClient   pbGiveaway.GiveawayServiceClient
	complianceClient pbCompliance.ComplianceServiceClient
	kycClient        pbKYC.KYCServiceClient
	paymentClient    pbPayment.PaymentRouterServiceClient
	auditClient      pbAudit.AuditServiceClient
	logger           *zap.Logger
	cfg              *config.Config
}

func NewAdminHandler(
	giveawayClient pbGiveaway.GiveawayServiceClient,
	complianceClient pbCompliance.ComplianceServiceClient,
	kycClient pbKYC.KYCServiceClient,
	paymentClient pbPayment.PaymentRouterServiceClient,
	auditClient pbAudit.AuditServiceClient,
	logger *zap.Logger,
	cfg *config.Config,
) *AdminHandler {
	return &AdminHandler{
		giveawayClient:   giveawayClient,
		complianceClient: complianceClient,
		kycClient:        kycClient,
		paymentClient:    paymentClient,
		auditClient:      auditClient,
		logger:           logger,
		cfg:              cfg,
	}
}

func (h *AdminHandler) RegisterRoutes(r chi.Router) {
	// Simple auth middleware for admin could be added here
	r.Get("/health", h.HTTPHealthCheck)
	r.Get("/giveaways/{id}", h.HTTPGetGiveaway)
	r.Post("/giveaways/{id}/cancel", h.HTTPCancelGiveaway)
	r.Get("/audit", h.HTTPQueryAudit)
}

func (h *AdminHandler) HTTPHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (h *AdminHandler) HTTPGetGiveaway(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	g, err := h.giveawayClient.GetGiveaway(ctx, &pbGiveaway.GiveawayIDRequest{Id: id})
	if err != nil {
		h.logger.Error("failed to get giveaway", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(g)
}

func (h *AdminHandler) HTTPCancelGiveaway(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Reason == "" {
		body.Reason = "Cancelled by admin"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	g, err := h.giveawayClient.CancelGiveaway(ctx, &pbGiveaway.CancelRequest{
		Id:     id,
		Reason: body.Reason,
	})
	if err != nil {
		h.logger.Error("failed to cancel giveaway", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(g)
}

func (h *AdminHandler) HTTPQueryAudit(w http.ResponseWriter, r *http.Request) {
	entityType := r.URL.Query().Get("entity_type")
	entityId := r.URL.Query().Get("entity_id")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resp, err := h.auditClient.QueryEvents(ctx, &pbAudit.QueryEventsRequest{
		EntityType: entityType,
		EntityId:   entityId,
		Limit:      50,
	})
	if err != nil {
		h.logger.Error("failed to query audit logs", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
