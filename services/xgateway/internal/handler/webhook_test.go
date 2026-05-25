package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/mosesedem/bot-x/shared/config"
	"go.uber.org/zap"
)

func TestHandleCRC(t *testing.T) {
    // Build handler with known consumer secret
    cfg := &config.Config{XConsumerSecret: "secret123"}
    logger, _ := zap.NewDevelopment()
    defer logger.Sync()
    handler := NewXWebhookHandler(asynq.NewClient(asynq.RedisClientOpt{Addr: "127.0.0.1:6379"}), cfg, logger)

    req := httptest.NewRequest("GET", "/webhooks/x/crc?crc_token=testtoken", nil)
    rr := httptest.NewRecorder()
    handler.HandleCRC(rr, req)

    if rr.Code != http.StatusOK {
        t.Fatalf("expected 200 OK, got %d", rr.Code)
    }
    body := rr.Body.String()
    if body == "" {
        t.Fatalf("expected non-empty response body")
    }
}
