package flutterwave

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

type Config struct {
	BaseURL   string
	SecretKey string
}

type Client struct {
	config Config
}

func New(cfg Config) *Client {
	return &Client{config: cfg}
}

type TransferRequest struct {
	AccountBank   string  `json:"account_bank"`
	AccountNumber string  `json:"account_number"`
	Amount        float64 `json:"amount"`
	Narration     string  `json:"narration"`
	Currency      string  `json:"currency"`
	Reference     string  `json:"reference"`
	CallbackURL   string  `json:"callback_url"`
}

type TransferResponse struct {
	ID        int     `json:"id"`
	Status    string  `json:"status"`
	Reference string  `json:"reference"`
	Amount    float64 `json:"amount"`
}

func (c *Client) Transfer(ctx context.Context, req TransferRequest) (*TransferResponse, error) {
	return nil, errors.New("flutterwave: not implemented")
}

func VerifyWebhookSignature(payload []byte, hash string, secret string) bool {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	expectedSignature := hex.EncodeToString(h.Sum(nil))
	return hmac.Equal([]byte(hash), []byte(expectedSignature))
}
