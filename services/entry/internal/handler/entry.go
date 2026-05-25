package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	pb "github.com/instantf/bot-x/gen/go/entry/v1"
	"github.com/instantf/bot-x/shared/fraud"
)

type EntryHandler struct {
	pb.UnimplementedEntryServiceServer
	db *pgxpool.Pool
}

func NewEntryHandler(db *pgxpool.Pool) *EntryHandler {
	return &EntryHandler{db: db}
}

func (h *EntryHandler) ScoreParticipant(ctx context.Context, req *pb.ScoreParticipantRequest) (*pb.ScoreParticipantResponse, error) {
	if req.Metadata == nil {
		return nil, fmt.Errorf("metadata is required to score participant")
	}

	score := fraud.Score(fraud.AccountMetadata{
		TwitterID:       req.Metadata.TwitterId,
		Handle:          req.Metadata.Handle,
		AccountAgeDays:  int(req.Metadata.AccountAgeDays),
		FollowerCount:   int(req.Metadata.FollowerCount),
		FollowingCount:  int(req.Metadata.FollowingCount),
		TweetCount:      int(req.Metadata.TweetCount),
		IsVerified:      req.Metadata.IsVerified,
		HasProfileImage: req.Metadata.HasProfileImage,
		PhoneVerified:   req.Metadata.PhoneVerified,
	})

	eligible := fraud.IsEligible(score, fraud.DefaultThreshold)
	rejectReason := ""
	if !eligible {
		rejectReason = "trust score below threshold"
	}

	return &pb.ScoreParticipantResponse{
		Score:        score,
		Eligible:     eligible,
		RejectReason: rejectReason,
	}, nil
}

func (h *EntryHandler) RegisterEntry(ctx context.Context, req *pb.RegisterEntryRequest) (*pb.RegisterEntryResponse, error) {
	if req.AccountMetadata == nil {
		return nil, fmt.Errorf("account metadata is required to register entry")
	}

	// 1. Check if the giveaway is ACTIVE
	var status string
	err := h.db.QueryRow(ctx, "SELECT status FROM giveaways WHERE id = $1", req.GiveawayId).Scan(&status)
	if err != nil {
		return nil, fmt.Errorf("failed to query giveaway: %w", err)
	}

	if strings.ToUpper(status) != "ACTIVE" {
		return &pb.RegisterEntryResponse{
			Accepted:     false,
			RejectReason: fmt.Sprintf("giveaway is not active (status: %s)", status),
		}, nil
	}

	// 2. Score participant
	scoreRes, err := h.ScoreParticipant(ctx, &pb.ScoreParticipantRequest{
		TwitterId: req.TwitterId,
		Metadata:  req.AccountMetadata,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to score participant: %w", err)
	}

	// 3. Save entry in DB
	var entryID string
	err = h.db.QueryRow(ctx, `
		INSERT INTO giveaway_entries (giveaway_id, twitter_id, twitter_handle, entry_type, trust_score, is_eligible, reject_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, req.GiveawayId, req.TwitterId, req.TwitterHandle, req.EntryType, scoreRes.Score, scoreRes.Eligible, scoreRes.RejectReason).Scan(&entryID)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Unique violation
			return &pb.RegisterEntryResponse{
				Accepted:     false,
				RejectReason: "already registered for this giveaway",
			}, nil
		}
		return nil, fmt.Errorf("failed to register entry: %w", err)
	}

	return &pb.RegisterEntryResponse{
		EntryId:      entryID,
		Accepted:     scoreRes.Eligible,
		RejectReason: scoreRes.RejectReason,
		TrustScore:   scoreRes.Score,
	}, nil
}

func (h *EntryHandler) GetEntryCount(ctx context.Context, req *pb.GetEntryCountRequest) (*pb.GetEntryCountResponse, error) {
	var count int64
	err := h.db.QueryRow(ctx, "SELECT count(*) FROM giveaway_entries WHERE giveaway_id = $1", req.GiveawayId).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed to count entries: %w", err)
	}
	return &pb.GetEntryCountResponse{Count: count}, nil
}

func (h *EntryHandler) GetEligibleEntries(ctx context.Context, req *pb.GetEligibleEntriesRequest) (*pb.GetEligibleEntriesResponse, error) {
	rows, err := h.db.Query(ctx, `
		SELECT id, twitter_id, twitter_handle, trust_score 
		FROM giveaway_entries 
		WHERE giveaway_id = $1 AND is_eligible = TRUE
	`, req.GiveawayId)
	if err != nil {
		return nil, fmt.Errorf("failed to query eligible entries: %w", err)
	}
	defer rows.Close()

	var entries []*pb.Entry
	for rows.Next() {
		var entry pb.Entry
		if err := rows.Scan(&entry.Id, &entry.TwitterId, &entry.TwitterHandle, &entry.TrustScore); err != nil {
			return nil, fmt.Errorf("failed to scan entry row: %w", err)
		}
		entries = append(entries, &entry)
	}

	return &pb.GetEligibleEntriesResponse{Entries: entries}, nil
}
