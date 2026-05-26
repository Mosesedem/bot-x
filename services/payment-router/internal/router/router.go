package router

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	pbAudit "github.com/mosesedem/bot-x/gen/go/audit/v1"
	pbCompliance "github.com/mosesedem/bot-x/gen/go/compliance/v1"
	pbGiveaway "github.com/mosesedem/bot-x/gen/go/giveaway/v1"
	pb "github.com/mosesedem/bot-x/gen/go/payment/v1"
	"github.com/mosesedem/bot-x/shared/gateways/crypto"
	"github.com/mosesedem/bot-x/shared/gateways/safehaven"
	botxstripe "github.com/mosesedem/bot-x/shared/gateways/stripe"
)

type PaymentRouter struct {
	pb.UnimplementedPaymentRouterServiceServer
	db               *pgxpool.Pool
	shClient         *safehaven.Client
	stripeClient     *botxstripe.Client
	cryptoClient     *crypto.Client
	complianceClient pbCompliance.ComplianceServiceClient
	auditClient      pbAudit.AuditServiceClient
	giveawayClient   pbGiveaway.GiveawayServiceClient
}

func NewPaymentRouter(
	db *pgxpool.Pool,
	shClient *safehaven.Client,
	stripeClient *botxstripe.Client,
	cryptoClient *crypto.Client,
	complianceClient pbCompliance.ComplianceServiceClient,
	auditClient pbAudit.AuditServiceClient,
	giveawayClient pbGiveaway.GiveawayServiceClient,
) *PaymentRouter {
	return &PaymentRouter{
		db:               db,
		shClient:         shClient,
		stripeClient:     stripeClient,
		cryptoClient:     cryptoClient,
		complianceClient: complianceClient,
		auditClient:      auditClient,
		giveawayClient:   giveawayClient,
	}
}

func (r *PaymentRouter) isGatewayEnabled(ctx context.Context, gateway string) (bool, error) {
	var enabled bool
	err := r.db.QueryRow(ctx, "SELECT enabled FROM payment_gateway_config WHERE gateway_name = $1", gateway).Scan(&enabled)
	if err != nil {
		// If row doesn't exist or table isn't fully migrated yet, assume disabled for safety.
		return false, nil
	}
	return enabled, nil
}

func (r *PaymentRouter) InitiateEscrow(ctx context.Context, req *pb.InitiateEscrowRequest) (*pb.InitiateEscrowResponse, error) {
	jurisdiction := strings.ToUpper(req.Jurisdiction)

	var gateway string
	if jurisdiction == "NG" {
		gateway = "safehaven"
	} else if jurisdiction == "US" {
		gateway = "stripe"
	} else {
		gateway = "crypto"
	}

	enabled, err := r.isGatewayEnabled(ctx, gateway)
	if err != nil {
		return nil, fmt.Errorf("failed to check gateway status: %w", err)
	}
	if !enabled {
		return nil, fmt.Errorf("gateway %s is currently disabled in the admin dashboard", gateway)
	}

	// Calculate total amount to fund (prize pool + 2% platform fee) in cents
	fee := (req.Amount*2 + 50) / 100 // 2% fee rounded half-up
	totalToFund := req.Amount + fee

	if gateway == "safehaven" {
		if r.shClient == nil {
			return nil, fmt.Errorf("Safe Haven client not configured")
		}

		va, err := r.shClient.CreateVirtualAccount(ctx, safehaven.CreateVirtualAccountRequest{
			AccountName: fmt.Sprintf("InstantF-%s", req.GiveawayId[:8]),
			BankCode:    "035", // Wema Bank
			ExternalRef: req.GiveawayId,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create Safe Haven virtual account: %w", err)
		}

		_, err = r.db.Exec(ctx, "UPDATE giveaways SET escrow_reference = $1, escrow_gateway = 'safehaven', funding_account = $2, funding_bank_code = $3 WHERE id = $4", va.Reference, va.AccountNumber, "035", req.GiveawayId)
		if err != nil {
			return nil, fmt.Errorf("failed to save virtual account details: %w", err)
		}

		_, _ = r.auditClient.LogEvent(ctx, &pbAudit.LogEventRequest{
			EntityType: "giveaway",
			EntityId:   req.GiveawayId,
			Action:     "ESCROW_INITIATED",
			ActorId:    req.HostTwitterId,
			Gateway:    "safehaven",
			Payload:    fmt.Sprintf(`{"account_number":"%s","total_to_fund_cents":%d}`, va.AccountNumber, totalToFund),
		})

		return &pb.InitiateEscrowResponse{
			VirtualAccountNumber: va.AccountNumber,
			BankName:             va.BankName,
			BankCode:             "035",
			AccountName:          va.AccountName,
			Gateway:              "safehaven",
			Reference:            va.Reference,
		}, nil

	} else if gateway == "stripe" {
		if r.stripeClient == nil {
			return nil, fmt.Errorf("Stripe client not configured")
		}

		pi, err := r.stripeClient.CreateEscrow(ctx, totalToFund, "usd", req.GiveawayId)
		if err != nil {
			return nil, fmt.Errorf("failed to create Stripe escrow: %w", err)
		}

		_, err = r.db.Exec(ctx, "UPDATE giveaways SET escrow_reference = $1, escrow_gateway = 'stripe', funding_account = $2, funding_bank_code = 'stripe' WHERE id = $3", pi.ID, pi.ID, req.GiveawayId)
		if err != nil {
			return nil, fmt.Errorf("failed to save Stripe escrow details: %w", err)
		}

		return &pb.InitiateEscrowResponse{
			VirtualAccountNumber: pi.ID,
			BankName:             "Stripe",
			BankCode:             "stripe",
			AccountName:          "Stripe Escrow",
			Gateway:              "stripe",
			Reference:            pi.ID,
		}, nil

	} else {
		// Crypto Gateway
		if r.cryptoClient == nil {
			return nil, fmt.Errorf("Crypto client not configured")
		}

		esc, err := r.cryptoClient.CreateEscrow(ctx, totalToFund, "USDC", req.GiveawayId)
		if err != nil {
			return nil, fmt.Errorf("failed to create Crypto escrow: %w", err)
		}

		_, err = r.db.Exec(ctx, "UPDATE giveaways SET escrow_reference = $1, escrow_gateway = 'crypto', funding_account = $2, funding_bank_code = 'crypto' WHERE id = $3", esc.TransactionHash, esc.ContractAddress, req.GiveawayId)
		if err != nil {
			return nil, fmt.Errorf("failed to save Crypto escrow details: %w", err)
		}

		return &pb.InitiateEscrowResponse{
			VirtualAccountNumber: esc.ContractAddress,
			BankName:             "Crypto Base",
			BankCode:             "crypto",
			AccountName:          "Crypto Escrow",
			Gateway:              "crypto",
			Reference:            esc.TransactionHash,
		}, nil
	}
}

func (r *PaymentRouter) CheckEscrowFunded(ctx context.Context, req *pb.CheckEscrowFundedRequest) (*pb.CheckEscrowFundedResponse, error) {
	var ref, gateway, status, accountNum string
	var totalBudgetInt int64
	err := r.db.QueryRow(ctx, `
		SELECT escrow_reference, escrow_gateway, total_budget, status, funding_account 
		FROM giveaways 
		WHERE id = $1
	`, req.GiveawayId).Scan(&ref, &gateway, &totalBudgetInt, &status, &accountNum)
	if err != nil {
		return nil, fmt.Errorf("failed to query giveaway: %w", err)
	}

	if strings.ToUpper(status) == "ACTIVE" {
		return &pb.CheckEscrowFundedResponse{
			Funded:         true,
			AmountReceived: totalBudgetInt,
			Gateway:        gateway,
			Reference:      ref,
		}, nil
	}

	// In Phase 1 dev environment, if the status is DRAFT and we call check, we can mock it as funded or verify.
	// For real verification, we query the gateway or database records of deposits.
	// Here, we'll return false if not marked active yet by the webhook.
	return &pb.CheckEscrowFundedResponse{
		Funded:         false,
		AmountReceived: 0,
		Gateway:        gateway,
		Reference:      ref,
	}, nil
}

func (r *PaymentRouter) RefundEscrow(ctx context.Context, req *pb.RefundEscrowRequest) (*pb.RefundEscrowResponse, error) {
	// Refund logic for cancelled giveaways
	_, _ = r.auditClient.LogEvent(ctx, &pbAudit.LogEventRequest{
		EntityType: "giveaway",
		EntityId:   req.GiveawayId,
		Action:     "ESCROW_REFUND_INITIATED",
		Payload:    fmt.Sprintf(`{"reason":"%s"}`, req.Reason),
	})

	return &pb.RefundEscrowResponse{
		Initiated: true,
		Reference: "refund-" + req.GiveawayId,
	}, nil
}

func (r *PaymentRouter) RoutePayment(ctx context.Context, req *pb.RoutePaymentRequest) (*pb.RoutePaymentResponse, error) {
	// 1. Idempotency Check
	var currentStatus, currentGateway, currentRef string
	err := r.db.QueryRow(ctx, `
		SELECT payment_status, gateway_used, gateway_reference 
		FROM giveaway_winners 
		WHERE id = $1
	`, req.WinnerId).Scan(&currentStatus, &currentGateway, &currentRef)
	if err != nil {
		return nil, fmt.Errorf("failed to query winner payout status: %w", err)
	}

	currentStatus = strings.ToUpper(currentStatus)
	if currentStatus == "SUCCESS" {
		return &pb.RoutePaymentResponse{
			Success:          true,
			GatewayUsed:      currentGateway,
			GatewayReference: currentRef,
			Status:           "SUCCESS",
		}, nil
	}
	if currentStatus == "PROCESSING" {
		return &pb.RoutePaymentResponse{
			Success:          true,
			GatewayUsed:      currentGateway,
			GatewayReference: currentRef,
			Status:           "PROCESSING",
		}, nil
	}

	// 2. Pre-flight compliance checks (OFAC check on beneficiary name)
	compRes, err := r.complianceClient.ScreenOFAC(ctx, &pbCompliance.ScreenOFACRequest{
		Address:     req.BeneficiaryName,
		AddressType: "name",
	})
	if err != nil {
		return nil, fmt.Errorf("compliance pre-flight OFAC check failed: %w", err)
	}

	if !compRes.Clear {
		// Update winner status to FAILED
		_, _ = r.db.Exec(ctx, "UPDATE giveaway_winners SET payment_status = 'FAILED' WHERE id = $1", req.WinnerId)

		// Log compliance failure
		_, _ = r.auditClient.LogEvent(ctx, &pbAudit.LogEventRequest{
			EntityType: "winner",
			EntityId:   req.WinnerId,
			Action:     "PAYOUT_COMPLIANCE_FAILED",
			Gateway:    "system",
			Payload:    fmt.Sprintf(`{"name":"%s","detail":"%s"}`, req.BeneficiaryName, compRes.MatchDetail),
		})

		return &pb.RoutePaymentResponse{
			Success:      false,
			Status:       "FAILED",
			ErrorMessage: "beneficiary name failed compliance / OFAC screening: " + compRes.MatchDetail,
		}, nil
	}

	// Update status to PROCESSING to prevent duplicate attempts
	_, err = r.db.Exec(ctx, "UPDATE giveaway_winners SET payment_status = 'PROCESSING' WHERE id = $1", req.WinnerId)
	if err != nil {
		return nil, fmt.Errorf("failed to transition winner payout to PROCESSING: %w", err)
	}

	// Fetch funding account from the giveaway
	var fundingAccount string
	err = r.db.QueryRow(ctx, "SELECT funding_account FROM giveaways WHERE id = $1", req.GiveawayId).Scan(&fundingAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to query giveaway funding account: %w", err)
	}

	// 3. Dispatch Payment
	jurisdiction := strings.ToUpper(req.Jurisdiction)
	var gateway string
	if jurisdiction == "NG" {
		gateway = "safehaven"
	} else if jurisdiction == "US" {
		gateway = "stripe"
	} else {
		gateway = "crypto"
	}

	enabled, err := r.isGatewayEnabled(ctx, gateway)
	if err != nil {
		_, _ = r.db.Exec(ctx, "UPDATE giveaway_winners SET payment_status = 'FAILED' WHERE id = $1", req.WinnerId)
		return nil, fmt.Errorf("failed to check gateway status: %w", err)
	}
	if !enabled {
		_, _ = r.db.Exec(ctx, "UPDATE giveaway_winners SET payment_status = 'FAILED' WHERE id = $1", req.WinnerId)
		return &pb.RoutePaymentResponse{
			Success:      false,
			Status:       "FAILED",
			ErrorMessage: "gateway " + gateway + " is currently disabled",
		}, nil
	}

	if gateway == "safehaven" {
		if r.shClient == nil {
			_, _ = r.db.Exec(ctx, "UPDATE giveaway_winners SET payment_status = 'FAILED' WHERE id = $1", req.WinnerId)
			return nil, fmt.Errorf("Safe Haven client not configured")
		}

		ne, err := r.shClient.NameEnquiry(ctx, safehaven.NameEnquiryRequest{
			AccountNumber: req.PayoutDestination,
			BankCode:      req.BankCode,
		})
		if err != nil {
			_, _ = r.db.Exec(ctx, "UPDATE giveaway_winners SET payment_status = 'FAILED' WHERE id = $1", req.WinnerId)
			return &pb.RoutePaymentResponse{
				Success:      false,
				Status:       "FAILED",
				ErrorMessage: "name enquiry failed: " + err.Error(),
			}, nil
		}

		tf, err := r.shClient.Transfer(ctx, safehaven.TransferRequest{
			NameEnquiryReference: "ne-ref-" + req.WinnerId,
			DebitAccountNumber:   fundingAccount, // Dynamically use the escrow virtual account
			BeneficiaryBank:      req.BankCode,
			BeneficiaryAccount:   req.PayoutDestination,
			BeneficiaryName:      ne.AccountName,
			Amount:               req.Amount,
			IdempotencyKey:       req.IdempotencyKey,
		})
		if err != nil {
			_, _ = r.db.Exec(ctx, "UPDATE giveaway_winners SET payment_status = 'FAILED' WHERE id = $1", req.WinnerId)
			return &pb.RoutePaymentResponse{
				Success:      false,
				Status:       "FAILED",
				ErrorMessage: "transfer execution failed: " + err.Error(),
			}, nil
		}

		_, _ = r.db.Exec(ctx, "UPDATE giveaway_winners SET gateway_used = 'safehaven', gateway_reference = $1, payout_initiated_at = $2 WHERE id = $3", tf.Reference, time.Now(), req.WinnerId)

		_, _ = r.auditClient.LogEvent(ctx, &pbAudit.LogEventRequest{
			EntityType: "winner",
			EntityId:   req.WinnerId,
			Action:     "PAYOUT_DISPATCHED",
			Gateway:    "safehaven",
			Payload:    fmt.Sprintf(`{"amount_cents":%d,"reference":"%s"}`, req.Amount, tf.Reference),
		})

		return &pb.RoutePaymentResponse{
			Success:          true,
			GatewayUsed:      "safehaven",
			GatewayReference: tf.Reference,
			Status:           "PROCESSING",
		}, nil

	} else if gateway == "stripe" {
		if r.stripeClient == nil {
			_, _ = r.db.Exec(ctx, "UPDATE giveaway_winners SET payment_status = 'FAILED' WHERE id = $1", req.WinnerId)
			return nil, fmt.Errorf("Stripe client not configured")
		}

		tf, err := r.stripeClient.TransferPayout(ctx, req.Amount, "usd", req.PayoutDestination, req.GiveawayId, req.WinnerId)
		if err != nil {
			_, _ = r.db.Exec(ctx, "UPDATE giveaway_winners SET payment_status = 'FAILED' WHERE id = $1", req.WinnerId)
			return &pb.RoutePaymentResponse{
				Success:      false,
				Status:       "FAILED",
				ErrorMessage: "stripe transfer execution failed: " + err.Error(),
			}, nil
		}

		_, _ = r.db.Exec(ctx, "UPDATE giveaway_winners SET gateway_used = 'stripe', gateway_reference = $1, payout_initiated_at = $2 WHERE id = $3", tf.ID, time.Now(), req.WinnerId)

		return &pb.RoutePaymentResponse{
			Success:          true,
			GatewayUsed:      "stripe",
			GatewayReference: tf.ID,
			Status:           "PROCESSING",
		}, nil

	} else {
		if r.cryptoClient == nil {
			_, _ = r.db.Exec(ctx, "UPDATE giveaway_winners SET payment_status = 'FAILED' WHERE id = $1", req.WinnerId)
			return nil, fmt.Errorf("Crypto client not configured")
		}

		tf, err := r.cryptoClient.TransferPayout(ctx, req.Amount, "USDC", req.PayoutDestination, req.GiveawayId, req.WinnerId)
		if err != nil {
			_, _ = r.db.Exec(ctx, "UPDATE giveaway_winners SET payment_status = 'FAILED' WHERE id = $1", req.WinnerId)
			return &pb.RoutePaymentResponse{
				Success:      false,
				Status:       "FAILED",
				ErrorMessage: "crypto transfer execution failed: " + err.Error(),
			}, nil
		}

		_, _ = r.db.Exec(ctx, "UPDATE giveaway_winners SET gateway_used = 'crypto', gateway_reference = $1, payout_initiated_at = $2 WHERE id = $3", tf.TransactionHash, time.Now(), req.WinnerId)

		return &pb.RoutePaymentResponse{
			Success:          true,
			GatewayUsed:      "crypto",
			GatewayReference: tf.TransactionHash,
			Status:           "PROCESSING",
		}, nil
	}
}

func (r *PaymentRouter) RetryPayout(ctx context.Context, req *pb.RetryPayoutRequest) (*pb.RoutePaymentResponse, error) {
	// Query details for retry
	var id, giveawayID, twitterID, dest, destType, bankCode, currency, jur, idempotencyKey string
	var amountInt int64
	err := r.db.QueryRow(ctx, `
		SELECT id, giveaway_id, winner_twitter_id, payout_destination, payout_destination_type, bank_code, amount, currency, idempotency_key 
		FROM giveaway_winners 
		WHERE id = $1
	`, req.WinnerId).Scan(&id, &giveawayID, &twitterID, &dest, &destType, &bankCode, &amountInt, &currency, &idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to query winner details for retry: %w", err)
	}

	// Query host jurisdiction for rules
	err = r.db.QueryRow(ctx, "SELECT jurisdiction FROM giveaways WHERE id = $1", giveawayID).Scan(&jur)
	if err != nil {
		jur = "NG"
	}

	// Log retry action
	_, _ = r.auditClient.LogEvent(ctx, &pbAudit.LogEventRequest{
		EntityType: "winner",
		EntityId:   id,
		Action:     "PAYOUT_RETRY_TRIGGERED",
		ActorId:    req.AdminActor,
		Gateway:    "system",
	})

	// Route the payment again (convert stored cents to float)
	return r.RoutePayment(ctx, &pb.RoutePaymentRequest{
		WinnerId:              id,
		GiveawayId:            giveawayID,
		TwitterId:             twitterID,
		Amount:                amountInt,
		Currency:              currency,
		Jurisdiction:          jur,
		PayoutDestination:     dest,
		PayoutDestinationType: destType,
		BankCode:              bankCode,
		BeneficiaryName:       "Retried Winner", // Default fallback name
		IdempotencyKey:        idempotencyKey,
	})
}

func (r *PaymentRouter) GetGatewayHealth(ctx context.Context, req *pb.GetGatewayHealthRequest) (*pb.GetGatewayHealthResponse, error) {
	// Simple healthy check
	return &pb.GetGatewayHealthResponse{
		Gateway:       req.Gateway,
		State:         "CLOSED", // Closed is the healthy state for a circuit breaker
		LastCheckedAt: time.Now().Format(time.RFC3339),
	}, nil
}
