package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	pb "github.com/mosesedem/bot-x/gen/go/kyc/v1"
	"github.com/mosesedem/bot-x/shared/gateways/safehaven"
)

type KYCHandler struct {
	pb.UnimplementedKYCServiceServer
	db       *pgxpool.Pool
	shClient *safehaven.Client
}

func NewKYCHandler(db *pgxpool.Pool, shClient *safehaven.Client) *KYCHandler {
	return &KYCHandler{
		db:       db,
		shClient: shClient,
	}
}

func (h *KYCHandler) IsKYCRequired(ctx context.Context, req *pb.IsKYCRequiredRequest) (*pb.IsKYCRequiredResponse, error) {
	var enabled bool
	err := h.db.QueryRow(ctx, "SELECT kyc_enabled FROM kyc_config WHERE jurisdiction = $1", req.Jurisdiction).Scan(&enabled)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			return &pb.IsKYCRequiredResponse{Required: false}, nil
		}
		return nil, fmt.Errorf("failed to query kyc_config: %w", err)
	}
	return &pb.IsKYCRequiredResponse{Required: enabled}, nil
}

func (h *KYCHandler) GetKYCConfig(ctx context.Context, req *pb.GetKYCConfigRequest) (*pb.GetKYCConfigResponse, error) {
	rows, err := h.db.Query(ctx, "SELECT jurisdiction, kyc_enabled FROM kyc_config")
	if err != nil {
		return nil, fmt.Errorf("failed to query kyc_configs: %w", err)
	}
	defer rows.Close()

	var configs []*pb.KYCJurisdictionConfig
	for rows.Next() {
		var jurisdiction string
		var enabled bool
		if err := rows.Scan(&jurisdiction, &enabled); err != nil {
			return nil, fmt.Errorf("failed to scan kyc_config row: %w", err)
		}
		configs = append(configs, &pb.KYCJurisdictionConfig{
			Jurisdiction: jurisdiction,
			Enabled:      enabled,
		})
	}

	return &pb.GetKYCConfigResponse{Configs: configs}, nil
}

func (h *KYCHandler) InitiateKYC(ctx context.Context, req *pb.InitiateKYCRequest) (*pb.InitiateKYCResponse, error) {
	// Support Nigeria (safehaven) and USA (stripe - stubbed for now)
	jurisdiction := strings.ToUpper(req.Jurisdiction)
	if jurisdiction != "NG" {
		// Mock non-NG jurisdictions
		return &pb.InitiateKYCResponse{
			Reference: "mock-ref-" + req.WinnerId,
			Provider:  "stripe_identity",
			Status:    "PENDING",
		}, nil
	}

	if h.shClient == nil {
		return nil, fmt.Errorf("Safe Haven client is not configured")
	}

	// Call Safe Haven identity verification API
	resp, err := h.shClient.InitiateKYC(ctx, safehaven.InitiateKYCRequest{
		Type:        strings.ToLower(req.KycType), // bvn or nin
		Identifier:  req.Identifier,
		PhoneNumber: req.PhoneNumber,
		ExternalRef: req.WinnerId,
	})
	if err != nil {
		return nil, fmt.Errorf("Safe Haven KYC initiation failed: %w", err)
	}

	// Update the database
	_, err = h.db.Exec(ctx, `
		UPDATE giveaway_winners 
		SET kyc_status = 'PENDING', kyc_provider = 'safehaven', kyc_reference = $1 
		WHERE id = $2
	`, resp.Reference, req.WinnerId)
	if err != nil {
		return nil, fmt.Errorf("failed to update giveaway winner KYC details: %w", err)
	}

	return &pb.InitiateKYCResponse{
		Reference: resp.Reference,
		Provider:  "safehaven",
		Status:    resp.Status,
	}, nil
}

func (h *KYCHandler) ValidateKYC(ctx context.Context, req *pb.ValidateKYCRequest) (*pb.ValidateKYCResponse, error) {
	// First check if the winner ID is using safehaven
	var provider string
	err := h.db.QueryRow(ctx, "SELECT kyc_provider FROM giveaway_winners WHERE id = $1", req.WinnerId).Scan(&provider)
	if err != nil {
		return nil, fmt.Errorf("failed to query winner provider: %w", err)
	}

	if provider != "safehaven" {
		// Mock validation for other providers
		_, err = h.db.Exec(ctx, "UPDATE giveaway_winners SET kyc_status = 'APPROVED' WHERE id = $1", req.WinnerId)
		if err != nil {
			return nil, err
		}
		return &pb.ValidateKYCResponse{Status: "APPROVED", Message: "KYC verification successful"}, nil
	}

	if h.shClient == nil {
		return nil, fmt.Errorf("Safe Haven client is not configured")
	}

	resp, err := h.shClient.ValidateKYC(ctx, safehaven.ValidateKYCRequest{
		Reference: req.Reference,
		OTP:       req.Otp,
	})
	if err != nil {
		return nil, fmt.Errorf("Safe Haven KYC validation failed: %w", err)
	}

	status := "APPROVED"
	if strings.ToUpper(resp.Status) != "APPROVED" && strings.ToUpper(resp.Status) != "SUCCESS" {
		status = "REJECTED"
	}

	_, err = h.db.Exec(ctx, "UPDATE giveaway_winners SET kyc_status = $1 WHERE id = $2", status, req.WinnerId)
	if err != nil {
		return nil, fmt.Errorf("failed to update giveaway winner KYC status: %w", err)
	}

	return &pb.ValidateKYCResponse{
		Status:  status,
		Message: resp.Message,
	}, nil
}
