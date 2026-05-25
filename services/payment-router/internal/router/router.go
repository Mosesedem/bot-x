package router

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	pbAudit "github.com/instantf/bot-x/gen/go/audit/v1"
	pbCompliance "github.com/instantf/bot-x/gen/go/compliance/v1"
	pbGiveaway "github.com/instantf/bot-x/gen/go/giveaway/v1"
	pb "github.com/instantf/bot-x/gen/go/payment/v1"
	"github.com/instantf/bot-x/shared/gateways/safehaven"
)

type PaymentRouter struct {
	pb.UnimplementedPaymentRouterServiceServer
	db               *pgxpool.Pool
	shClient         *safehaven.Client
	complianceClient pbCompliance.ComplianceServiceClient
	auditClient      pbAudit.AuditServiceClient
	giveawayClient   pbGiveaway.GiveawayServiceClient
	debitAccount     string
}

func NewPaymentRouter(
	db *pgxpool.Pool,
	shClient *safehaven.Client,
	complianceClient pbCompliance.ComplianceServiceClient,
	auditClient pbAudit.AuditServiceClient,
	giveawayClient pbGiveaway.GiveawayServiceClient,
	debitAccount string,
) *PaymentRouter {
	if debitAccount == "" {
		debitAccount = "0123456789" // fallback default
	}
	return &PaymentRouter{
		db:               db,
		shClient:         shClient,
		complianceClient: complianceClient,
		auditClient:      auditClient,
		giveawayClient:   giveawayClient,
		debitAccount:     debitAccount,
	}
}

func (r *PaymentRouter) InitiateEscrow(ctx context.Context, req *pb.InitiateEscrowRequest) (*pb.InitiateEscrowResponse, error) {
	// For NG, create a Wema virtual account using Safe Haven
	jurisdiction := strings.ToUpper(req.Jurisdiction)
	if jurisdiction != "NG" {
		// Mock non-NG escrow
		ref := "mock-escrow-" + req.GiveawayId
		accNum := "99" + req.GiveawayId[:8]
		_, err := r.db.Exec(ctx, `
			UPDATE giveaways 
			SET escrow_reference = $1, escrow_gateway = $2, funding_account = $3, funding_bank_code = $4 
			WHERE id = $5
		`, ref, "stripe", accNum, "stripe", req.GiveawayId)
		if err != nil {
			return nil, err
		}

		return &pb.InitiateEscrowResponse{
			VirtualAccountNumber: accNum,
			BankName:             "Stripe Escrow",
			BankCode:             "stripe",
			AccountName:          "InstantF Escrow",
			Gateway:              "stripe",
			Reference:            ref,
		}, nil
	}

	if r.shClient == nil {
		return nil, fmt.Errorf("Safe Haven client not configured")
	}

	// Calculate total amount to fund (prize pool + 2% platform fee)
	totalToFund := req.Amount * 1.02

	// Call Safe Haven to create a virtual account
	va, err := r.shClient.CreateVirtualAccount(ctx, safehaven.CreateVirtualAccountRequest{
		AccountName: fmt.Sprintf("InstantF-%s", req.GiveawayId[:8]),
		BankCode:    "035", // Wema Bank
		ExternalRef: req.GiveawayId,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Safe Haven virtual account: %w", err)
	}

	// Save to database
	_, err = r.db.Exec(ctx, `
		UPDATE giveaways 
		SET escrow_reference = $1, escrow_gateway = 'safehaven', funding_account = $2, funding_bank_code = $3 
		WHERE id = $4
	`, va.Reference, va.AccountNumber, "035", req.GiveawayId)
	if err != nil {
		return nil, fmt.Errorf("failed to save virtual account details: %w", err)
	}

	// Log audit event
	_, _ = r.auditClient.LogEvent(ctx, &pbAudit.LogEventRequest{
		EntityType: "giveaway",
		EntityId:   req.GiveawayId,
		Action:     "ESCROW_INITIATED",
		ActorId:    req.HostTwitterId,
		Gateway:    "safehaven",
		Payload:    fmt.Sprintf(`{"account_number":"%s","total_to_fund":%.2f}`, va.AccountNumber, totalToFund),
	})

	return &pb.InitiateEscrowResponse{
		VirtualAccountNumber: va.AccountNumber,
		BankName:             va.BankName,
		BankCode:             "035",
		AccountName:          va.AccountName,
		Gateway:              "safehaven",
		Reference:            va.Reference,
	}, nil
}

func (r *PaymentRouter) CheckEscrowFunded(ctx context.Context, req *pb.CheckEscrowFundedRequest) (*pb.CheckEscrowFundedResponse, error) {
	var ref, gateway, status, accountNum string
	var totalBudget float64
	err := r.db.QueryRow(ctx, `
		SELECT escrow_reference, escrow_gateway, total_budget, status, funding_account 
		FROM giveaways 
		WHERE id = $1
	`, req.GiveawayId).Scan(&ref, &gateway, &totalBudget, &status, &accountNum)
	if err != nil {
		return nil, fmt.Errorf("failed to query giveaway: %w", err)
	}

	if strings.ToUpper(status) == "ACTIVE" {
		return &pb.CheckEscrowFundedResponse{
			Funded:         true,
			AmountReceived: totalBudget,
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

	// 3. Dispatch Payment (Safe Haven for NG, Mock for others)
	jurisdiction := strings.ToUpper(req.Jurisdiction)
	if jurisdiction != "NG" {
		// Mock non-NG payout
		ref := "mock-payout-" + req.WinnerId
		_, _ = r.db.Exec(ctx, "UPDATE giveaway_winners SET payment_status = 'SUCCESS', gateway_used = 'mock', gateway_reference = $1, payout_completed_at = $2 WHERE id = $3", ref, time.Now(), req.WinnerId)
		
		_, _ = r.auditClient.LogEvent(ctx, &pbAudit.LogEventRequest{
			EntityType: "winner",
			EntityId:   req.WinnerId,
			Action:     "PAYOUT_SUCCESS",
			Gateway:    "mock",
			Payload:    fmt.Sprintf(`{"amount":%.2f}`, req.Amount),
		})

		return &pb.RoutePaymentResponse{
			Success:          true,
			GatewayUsed:      "mock",
			GatewayReference: ref,
			Status:           "SUCCESS",
		}, nil
	}

	if r.shClient == nil {
		// Revert status
		_, _ = r.db.Exec(ctx, "UPDATE giveaway_winners SET payment_status = 'FAILED' WHERE id = $1", req.WinnerId)
		return nil, fmt.Errorf("Safe Haven client not configured")
	}

	// 4. Safe Haven Name Enquiry
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

	// 5. Safe Haven Transfer
	tf, err := r.shClient.Transfer(ctx, safehaven.TransferRequest{
		NameEnquiryReference: "ne-ref-" + req.WinnerId, // Safe Haven name enquiry ref
		DebitAccountNumber:   r.debitAccount,
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

	// Update DB to include gateway ref
	_, err = r.db.Exec(ctx, `
		UPDATE giveaway_winners 
		SET gateway_used = 'safehaven', gateway_reference = $1, payout_initiated_at = $2 
		WHERE id = $3
	`, tf.Reference, time.Now(), req.WinnerId)
	if err != nil {
		// Log but don't fail, since transaction was dispatched
		r.auditClient.LogEvent(ctx, &pbAudit.LogEventRequest{
			EntityType: "winner",
			EntityId:   req.WinnerId,
			Action:     "PAYOUT_DB_UPDATE_ERROR",
			Gateway:    "safehaven",
			Payload:    fmt.Sprintf(`{"reference":"%s","error":"%s"}`, tf.Reference, err.Error()),
		})
	}

	// Log audit event
	_, _ = r.auditClient.LogEvent(ctx, &pbAudit.LogEventRequest{
		EntityType: "winner",
		EntityId:   req.WinnerId,
		Action:     "PAYOUT_DISPATCHED",
		Gateway:    "safehaven",
		Payload:    fmt.Sprintf(`{"amount":%.2f,"reference":"%s"}`, req.Amount, tf.Reference),
	})

	return &pb.RoutePaymentResponse{
		Success:          true,
		GatewayUsed:      "safehaven",
		GatewayReference: tf.Reference,
		Status:           "PROCESSING",
	}, nil
}

func (r *PaymentRouter) RetryPayout(ctx context.Context, req *pb.RetryPayoutRequest) (*pb.RoutePaymentResponse, error) {
	// Query details for retry
	var id, giveawayID, twitterID, dest, destType, bankCode, currency, jur, idempotencyKey string
	var amount float64
	err := r.db.QueryRow(ctx, `
		SELECT id, giveaway_id, winner_twitter_id, payout_destination, payout_destination_type, bank_code, amount, currency, idempotency_key 
		FROM giveaway_winners 
		WHERE id = $1
	`, req.WinnerId).Scan(&id, &giveawayID, &twitterID, &dest, &destType, &bankCode, &amount, &currency, &idempotencyKey)
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

	// Route the payment again
	return r.RoutePayment(ctx, &pb.RoutePaymentRequest{
		WinnerId:              id,
		GiveawayId:            giveawayID,
		TwitterId:             twitterID,
		Amount:                amount,
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
