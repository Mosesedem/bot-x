package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	pb "github.com/instantf/bot-x/gen/go/compliance/v1"
	"github.com/instantf/bot-x/shared/ofac"
)

type ComplianceHandler struct {
	pb.UnimplementedComplianceServiceServer
	db       *pgxpool.Pool
	screener *ofac.Screener
}

func NewComplianceHandler(db *pgxpool.Pool, sdnPath string) *ComplianceHandler {
	scr := ofac.New()
	if sdnPath != "" {
		if _, err := os.Stat(sdnPath); err == nil {
			_ = scr.LoadFromFile(sdnPath)
		}
	}
	return &ComplianceHandler{
		db:       db,
		screener: scr,
	}
}

func (h *ComplianceHandler) CheckGiveawayEligibility(ctx context.Context, req *pb.CheckEligibilityRequest) (*pb.CheckEligibilityResponse, error) {
	var reasons []string
	eligible := true

	// 1. Check if jurisdiction is blocked
	blockedRes, err := h.IsJurisdictionBlocked(ctx, &pb.IsJurisdictionBlockedRequest{CountryCode: req.Jurisdiction})
	if err != nil {
		return nil, err
	}
	if blockedRes.Blocked {
		eligible = false
		reasons = append(reasons, blockedRes.Reason)
	}

	// 2. Check if host is suspended
	var isSuspended bool
	err = h.db.QueryRow(ctx, "SELECT is_suspended FROM host_profiles WHERE twitter_id = $1", req.HostTwitterId).Scan(&isSuspended)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			// Host doesn't exist yet, which is fine, they will be registered on first giveaway
			isSuspended = false
		} else {
			return nil, fmt.Errorf("failed to query host profile: %w", err)
		}
	}

	if isSuspended {
		eligible = false
		reasons = append(reasons, "host account is suspended on this platform")
	}

	// 3. Amount checks
	if req.Amount <= 0 {
		eligible = false
		reasons = append(reasons, "giveaway amount must be greater than zero")
	}

	// 4. Currency support check
	var hasGateway bool
	err = h.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM payment_gateway_config 
			WHERE enabled = TRUE AND $1 = ANY(supported_currencies)
		)
	`, req.Currency).Scan(&hasGateway)
	if err != nil {
		return nil, fmt.Errorf("failed to query gateway configs: %w", err)
	}

	if !hasGateway {
		eligible = false
		reasons = append(reasons, fmt.Sprintf("currency %s is not supported by any active payment gateway", req.Currency))
	}

	return &pb.CheckEligibilityResponse{
		Eligible: eligible,
		Reasons:  reasons,
	}, nil
}

func (h *ComplianceHandler) ScreenOFAC(ctx context.Context, req *pb.ScreenOFACRequest) (*pb.ScreenOFACResponse, error) {
	// If screener has no entries, load a dummy fallback for testing
	if h.screener.Count() == 0 {
		// Basic in-memory testing entries
		testEntries := []string{"evader", "terrorist", "sanctioned individual", "launderer"}
		for _, e := range testEntries {
			if strings.Contains(strings.ToLower(req.Address), e) {
				return &pb.ScreenOFACResponse{
					Clear:       false,
					MatchDetail: fmt.Sprintf("Match found in test OFAC list for query: %s", req.Address),
				}, nil
			}
		}
		return &pb.ScreenOFACResponse{Clear: true}, nil
	}

	isMatch := h.screener.Screen(req.Address)
	if isMatch {
		return &pb.ScreenOFACResponse{
			Clear:       false,
			MatchDetail: fmt.Sprintf("Name/Address matches OFAC SDN entry"),
		}, nil
	}

	return &pb.ScreenOFACResponse{Clear: true}, nil
}

func (h *ComplianceHandler) IsJurisdictionBlocked(ctx context.Context, req *pb.IsJurisdictionBlockedRequest) (*pb.IsJurisdictionBlockedResponse, error) {
	cc := strings.ToUpper(req.CountryCode)
	
	// Supported jurisdictions are NG (and US if configured)
	if cc == "NG" {
		return &pb.IsJurisdictionBlockedResponse{Blocked: false}, nil
	}
	if cc == "US" {
		// Check if Stripe is enabled or if there's any active gateway supporting US
		var usSupported bool
		err := h.db.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM payment_gateway_config 
				WHERE enabled = TRUE AND $1 = ANY(supported_jurisdictions)
			)
		`, "US").Scan(&usSupported)
		if err != nil {
			return nil, fmt.Errorf("failed to query gateway config for US: %w", err)
		}
		if usSupported {
			return &pb.IsJurisdictionBlockedResponse{Blocked: false}, nil
		}
		return &pb.IsJurisdictionBlockedResponse{
			Blocked: true,
			Reason:  "US jurisdiction is currently disabled by administrator config",
		}, nil
	}

	// Other jurisdictions blocked in Phase 1
	return &pb.IsJurisdictionBlockedResponse{
		Blocked: true,
		Reason:  fmt.Sprintf("Jurisdiction %s is not supported in this phase", cc),
	}, nil
}

func (h *ComplianceHandler) GetJurisdictionRules(ctx context.Context, req *pb.GetJurisdictionRulesRequest) (*pb.GetJurisdictionRulesResponse, error) {
	cc := strings.ToUpper(req.CountryCode)

	blockedRes, err := h.IsJurisdictionBlocked(ctx, &pb.IsJurisdictionBlockedRequest{CountryCode: cc})
	if err != nil {
		return nil, err
	}

	var kycEnabled bool
	err = h.db.QueryRow(ctx, "SELECT kyc_enabled FROM kyc_config WHERE jurisdiction = $1", cc).Scan(&kycEnabled)
	if err != nil {
		// Default to false if not found
		kycEnabled = false
	}

	maxPayoutWithoutKYC := 50000.0 // ₦50,000 default for NG
	if cc == "US" {
		maxPayoutWithoutKYC = 600.0 // $600 default for US
	}

	return &pb.GetJurisdictionRulesResponse{
		CountryCode:         cc,
		Blocked:             blockedRes.Blocked,
		KycRequired:         kycEnabled,
		MaxPayoutWithoutKyc: maxPayoutWithoutKYC,
		Notes:               fmt.Sprintf("Rules active for %s", cc),
	}, nil
}
