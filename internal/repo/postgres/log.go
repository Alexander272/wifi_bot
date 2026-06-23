package postgres

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"wifi_bot/internal/models"
)

type LogRepo struct {
	db *sqlx.DB
}

func NewLogRepo(db *sqlx.DB) *LogRepo {
	return &LogRepo{db: db}
}

func (r *LogRepo) Create(ctx context.Context, entry *models.AuthLog) error {
	query := `INSERT INTO auth_logs (user_id, action, code, mac, ip, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.db.ExecContext(ctx, query,
		entry.UserID, entry.Action, nullString(entry.Code),
		nullString(entry.Mac), nullString(entry.IP), nullString(entry.Metadata),
	)
	if err != nil {
		return fmt.Errorf("failed to insert log: %w", err)
	}
	return nil
}

func (r *LogRepo) GetByUser(ctx context.Context, userID string, limit, offset int) ([]models.AuthLog, error) {
	query := `SELECT id, user_id, action, code, mac, ip, metadata, created_at
		FROM auth_logs WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	var logs []models.AuthLog
	if err := r.db.SelectContext(ctx, &logs, query, userID, limit, offset); err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}
	return logs, nil
}

func (r *LogRepo) GetAll(ctx context.Context, limit, offset int) ([]models.AuthLog, error) {
	query := `SELECT id, user_id, action, code, mac, ip, metadata, created_at
		FROM auth_logs ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	var logs []models.AuthLog
	if err := r.db.SelectContext(ctx, &logs, query, limit, offset); err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}
	return logs, nil
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
