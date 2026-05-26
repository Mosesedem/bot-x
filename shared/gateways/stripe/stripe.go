package stripe

import (
	"context"
	"fmt"

	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/paymentintent"
	"github.com/stripe/stripe-go/v78/transfer"
)

type Client struct {
	secretKey string
}

func New(secretKey string) *Client {
	stripe.Key = secretKey
	return &Client{
		secretKey: secretKey,
	}
}

// CreateEscrow creates a PaymentIntent for the giveaway pool.
func (c *Client) CreateEscrow(ctx context.Context, amount int64, currency, giveawayID string) (*stripe.PaymentIntent, error) {
	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(amount),
		Currency: stripe.String(currency),
		Metadata: map[string]string{
			"giveaway_id": giveawayID,
			"purpose":     "escrow",
		},
	}
	// For context propagation
	params.Context = ctx

	pi, err := paymentintent.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create Stripe PaymentIntent: %w", err)
	}

	return pi, nil
}

// TransferPayout transfers funds to a connected account (winner).
func (c *Client) TransferPayout(ctx context.Context, amount int64, currency, destinationAccount, giveawayID, winnerID string) (*stripe.Transfer, error) {
	params := &stripe.TransferParams{
		Amount:      stripe.Int64(amount),
		Currency:    stripe.String(currency),
		Destination: stripe.String(destinationAccount),
		Metadata: map[string]string{
			"giveaway_id": giveawayID,
			"winner_id":   winnerID,
			"purpose":     "payout",
		},
	}
	params.Context = ctx

	t, err := transfer.New(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create Stripe transfer: %w", err)
	}

	return t, nil
}
