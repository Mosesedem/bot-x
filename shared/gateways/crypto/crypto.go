package crypto

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type Client struct {
	rpcURL  string
	chainID int
}

func New(rpcURL string, chainID int) *Client {
	return &Client{
		rpcURL:  rpcURL,
		chainID: chainID,
	}
}

type EscrowResponse struct {
	ContractAddress string
	TransactionHash string
}

type PayoutResponse struct {
	TransactionHash string
}

func syntheticReference(prefix string, parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "|")))
	encoded := hex.EncodeToString(hash[:])
	if len(encoded) > 24 {
		encoded = encoded[:24]
	}
	return fmt.Sprintf("%s_%s", prefix, encoded)
}

func syntheticAddress(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "|")))
	encoded := hex.EncodeToString(hash[:])
	if len(encoded) > 40 {
		encoded = encoded[:40]
	}
	return "0x" + encoded
}

// CreateEscrow simulates creating an escrow contract or transferring to a treasury vault.
func (c *Client) CreateEscrow(ctx context.Context, amount int64, token, giveawayID string) (*EscrowResponse, error) {
	if c.rpcURL == "" {
		return nil, fmt.Errorf("crypto RPC URL not configured")
	}
	if amount <= 0 {
		return nil, fmt.Errorf("crypto escrow amount must be positive")
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("crypto escrow token is required")
	}
	if strings.TrimSpace(giveawayID) == "" {
		return nil, fmt.Errorf("crypto escrow giveaway ID is required")
	}

	return &EscrowResponse{
		ContractAddress: syntheticAddress(c.rpcURL, fmt.Sprint(c.chainID), token, giveawayID, fmt.Sprint(amount)),
		TransactionHash: syntheticReference("0xescrow", c.rpcURL, fmt.Sprint(c.chainID), token, giveawayID, fmt.Sprint(amount)),
	}, nil
}

// TransferPayout simulates transferring crypto to a winner's wallet address.
func (c *Client) TransferPayout(ctx context.Context, amount int64, token, destinationWallet, giveawayID, winnerID string) (*PayoutResponse, error) {
	if c.rpcURL == "" {
		return nil, fmt.Errorf("crypto RPC URL not configured")
	}
	if amount <= 0 {
		return nil, fmt.Errorf("crypto payout amount must be positive")
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("crypto payout token is required")
	}
	if strings.TrimSpace(destinationWallet) == "" {
		return nil, fmt.Errorf("crypto payout destination wallet is required")
	}
	if strings.TrimSpace(giveawayID) == "" {
		return nil, fmt.Errorf("crypto payout giveaway ID is required")
	}
	if strings.TrimSpace(winnerID) == "" {
		return nil, fmt.Errorf("crypto payout winner ID is required")
	}

	return &PayoutResponse{
		TransactionHash: syntheticReference("0xpay", c.rpcURL, fmt.Sprint(c.chainID), token, destinationWallet, giveawayID, winnerID, fmt.Sprint(amount)),
	}, nil
}
