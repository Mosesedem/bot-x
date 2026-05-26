package paystack

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
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

type RecipientRequest struct {
	Type          string `json:"type"` // nuban or mobile_money
	Name          string `json:"name"`
	AccountNumber string `json:"account_number"`
	BankCode      string `json:"bank_code"`
	Currency      string `json:"currency"`
}

type RecipientResponse struct {
	RecipientCode string `json:"recipient_code"`
	Name          string `json:"name"`
	Type          string `json:"type"`
}

type TransferRequest struct {
	Source    string `json:"source"` // balance
	Amount    int64  `json:"amount"` // in kobo/cents
	Recipient string `json:"recipient"`
	Reason    string `json:"reason"`
	Reference string `json:"reference"`
}

type TransferResponse struct {
	Reference    string `json:"reference"`
	Status       string `json:"status"`
	TransferCode string `json:"transfer_code"`
	Amount       int64  `json:"amount"`
}

func (c *Client) CreateTransferRecipient(ctx context.Context, req RecipientRequest) (*RecipientResponse, error) {
	return nil, errors.New("paystack: not implemented")
}

func (c *Client) Transfer(ctx context.Context, req TransferRequest) (*TransferResponse, error) {
	return nil, errors.New("paystack: not implemented")
}

func VerifyWebhookSignature(payload []byte, signature string, secret string) bool {
	h := hmac.New(sha512.New, []byte(secret))
	h.Write(payload)
	expectedSignature := hex.EncodeToString(h.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}
