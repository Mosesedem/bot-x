package handler

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	pb "github.com/mosesedem/bot-x/gen/go/audit/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type AuditHandler struct {
	pb.UnimplementedAuditServiceServer
	chConn clickhouse.Conn
}

func NewAuditHandler(chConn clickhouse.Conn) *AuditHandler {
	return &AuditHandler{chConn: chConn}
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (h *AuditHandler) LogEvent(ctx context.Context, req *pb.LogEventRequest) (*pb.LogEventResponse, error) {
	eventID := newUUID()
	createdAt := time.Now()

	err := h.chConn.Exec(ctx, `
		INSERT INTO audit_events (
			event_id, entity_type, entity_id, action, actor_id, gateway, payload, ip_address, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, eventID, req.EntityType, req.EntityId, req.Action, req.ActorId, req.Gateway, req.Payload, req.IpAddress, createdAt)

	if err != nil {
		return nil, fmt.Errorf("failed to insert audit event to ClickHouse: %w", err)
	}

	return &pb.LogEventResponse{
		EventId: eventID,
	}, nil
}

func (h *AuditHandler) QueryEvents(ctx context.Context, req *pb.QueryEventsRequest) (*pb.QueryEventsResponse, error) {
	query := "SELECT event_id, entity_type, entity_id, action, actor_id, gateway, payload, created_at FROM audit_events WHERE 1=1"
	var args []interface{}

	if req.EntityType != "" {
		query += " AND entity_type = ?"
		args = append(args, req.EntityType)
	}
	if req.EntityId != "" {
		query += " AND entity_id = ?"
		args = append(args, req.EntityId)
	}
	if req.Action != "" {
		query += " AND action = ?"
		args = append(args, req.Action)
	}
	if req.Gateway != "" {
		query += " AND gateway = ?"
		args = append(args, req.Gateway)
	}
	if req.From != nil {
		query += " AND created_at >= ?"
		args = append(args, req.From.AsTime())
	}
	if req.To != nil {
		query += " AND created_at <= ?"
		args = append(args, req.To.AsTime())
	}

	query += " ORDER BY created_at DESC"

	limit := int32(100)
	if req.Limit > 0 {
		limit = req.Limit
	}
	query += " LIMIT ?"
	args = append(args, limit)

	if req.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, req.Offset)
	}

	rows, err := h.chConn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query ClickHouse: %w", err)
	}
	defer rows.Close()

	var events []*pb.AuditEvent
	for rows.Next() {
		var event pb.AuditEvent
		var createdAt time.Time
		var eventID, entityType, entityID, action, actorID, gateway, payload string

		err := rows.Scan(
			&eventID,
			&entityType,
			&entityID,
			&action,
			&actorID,
			&gateway,
			&payload,
			&createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		event.EventId = eventID
		event.EntityType = entityType
		event.EntityId = entityID
		event.Action = action
		event.ActorId = actorID
		event.Gateway = gateway
		event.Payload = payload
		event.CreatedAt = timestamppb.New(createdAt)

		events = append(events, &event)
	}

	// For total count
	var total int64
	countQuery := "SELECT count() FROM audit_events"
	// Parse count query filters
	countQueryPart := " WHERE 1=1"
	var countArgs []interface{}
	if req.EntityType != "" {
		countQueryPart += " AND entity_type = ?"
		countArgs = append(countArgs, req.EntityType)
	}
	if req.EntityId != "" {
		countQueryPart += " AND entity_id = ?"
		countArgs = append(countArgs, req.EntityId)
	}
	if req.Action != "" {
		countQueryPart += " AND action = ?"
		countArgs = append(countArgs, req.Action)
	}
	if req.Gateway != "" {
		countQueryPart += " AND gateway = ?"
		countArgs = append(countArgs, req.Gateway)
	}
	if req.From != nil {
		countQueryPart += " AND created_at >= ?"
		countArgs = append(countArgs, req.From.AsTime())
	}
	if req.To != nil {
		countQueryPart += " AND created_at <= ?"
		countArgs = append(countArgs, req.To.AsTime())
	}
	err = h.chConn.QueryRow(ctx, countQuery+countQueryPart, countArgs...).Scan(&total)
	if err != nil {
		// Just log and continue, don't fail the whole query if count fails
		total = int64(len(events))
	}

	return &pb.QueryEventsResponse{
		Events: events,
		Total:  int32(total),
	}, nil
}
