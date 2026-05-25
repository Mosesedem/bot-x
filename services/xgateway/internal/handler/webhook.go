package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/instantf/bot-x/shared/config"
	"go.uber.org/zap"
)

type XWebhookHandler struct {
	asynqClient *asynq.Client
	cfg         *config.Config
	logger      *zap.Logger
}

func NewXWebhookHandler(asynqClient *asynq.Client, cfg *config.Config, logger *zap.Logger) *XWebhookHandler {
	return &XWebhookHandler{
		asynqClient: asynqClient,
		cfg:         cfg,
		logger:      logger,
	}
}

func (h *XWebhookHandler) RegisterRoutes(r chi.Router) {
	r.Get("/webhooks/x/crc", h.HandleCRC)
	r.Post("/webhooks/x/events", h.HandleEvents)
}

func (h *XWebhookHandler) HandleCRC(w http.ResponseWriter, r *http.Request) {
	crcToken := r.URL.Query().Get("crc_token")
	if crcToken == "" {
		http.Error(w, "missing crc_token", http.StatusBadRequest)
		return
	}

	// Calculate HMAC-SHA256 signature
	mac := hmac.New(sha256.New, []byte(h.cfg.XConsumerSecret))
	mac.Write([]byte(crcToken))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(fmt.Sprintf(`{"response_token":"sha256=%s"}`, sig)))
}

func (h *XWebhookHandler) HandleEvents(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed to read X webhook body", zap.Error(err))
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Verify X Webhook signature if configured
	signature := r.Header.Get("X-Twitter-Webhooks-Signature")
	if h.cfg.XConsumerSecret != "" && signature != "" {
		if !h.verifySignature(body, signature) {
			h.logger.Warn("invalid X webhook signature received")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Enqueue the event raw payload to Asynq queue for async processing
	task := asynq.NewTask("x_webhook_event", body)
	info, err := h.asynqClient.Enqueue(task, asynq.MaxRetry(3))
	if err != nil {
		h.logger.Error("failed to enqueue event to Asynq", zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	h.logger.Info("enqueued X event to Asynq", zap.String("task_id", info.ID))
	w.WriteHeader(http.StatusOK)
}

func (h *XWebhookHandler) verifySignature(body []byte, signatureHeader string) bool {
	// Format is typically "sha256=..."
	signature := signatureHeader
	if len(signatureHeader) > 7 && signatureHeader[:7] == "sha256=" {
		signature = signatureHeader[7:]
	}

	mac := hmac.New(sha256.New, []byte(h.cfg.XConsumerSecret))
	mac.Write(body)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedSig))
}

// Struct definitions to match X Account Activity API events
type TweetCreateEvent struct {
	IDStr        string `json:"id_str"`
	Text         string `json:"text"`
	User         XUser  `json:"user"`
	InReplyTo    string `json:"in_reply_to_status_id_str"`
	CreatedEvent bool   `json:"created_event"`
}

type XUser struct {
	IDStr      string `json:"id_str"`
	ScreenName string `json:"screen_name"`
}

type DirectMessageEvent struct {
	Type      string `json:"type"`
	MessageID string `json:"id"`
	Message   struct {
		SenderID string `json:"sender_id"`
		Text     string `json:"text"`
	} `json:"message_create"`
}
