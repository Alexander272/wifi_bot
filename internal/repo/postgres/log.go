package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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
	query := `INSERT INTO auth_logs (user_id, username, action, code, mac, ip, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.db.ExecContext(ctx, query,
		entry.UserID, entry.Username, entry.Action,
		entry.Code, entry.Mac, entry.IP, entry.Metadata,
	)
	if err != nil {
		return fmt.Errorf("failed to insert log: %w", err)
	}
	return nil
}

func (r *LogRepo) GetByUser(ctx context.Context, userID string, limit, offset int) ([]models.AuthLog, error) {
	query := `SELECT id, user_id, username, action, code, mac, ip, metadata, created_at
		FROM auth_logs WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	var logs []models.AuthLog
	if err := r.db.SelectContext(ctx, &logs, query, userID, limit, offset); err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}
	return logs, nil
}

func (r *LogRepo) GetAll(ctx context.Context, limit, offset int) ([]models.AuthLog, error) {
	query := `SELECT id, user_id, username, action, code, mac, ip, metadata, created_at
		FROM auth_logs ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	var logs []models.AuthLog
	if err := r.db.SelectContext(ctx, &logs, query, limit, offset); err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}
	return logs, nil
}

func (r *LogRepo) GetLastMACByUserID(ctx context.Context, userID string) (string, error) {
	query := `SELECT mac FROM auth_logs
		WHERE user_id = $1 AND action = 'code_used' AND mac IS NOT NULL AND mac != ''
		ORDER BY created_at DESC LIMIT 1`
	var mac string
	if err := r.db.GetContext(ctx, &mac, query, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get last mac: %w", err)
	}
	return mac, nil
}

func (r *LogRepo) CountByActionSince(ctx context.Context, action string, since, to time.Time) (int, error) {
	query := `SELECT COUNT(*) FROM auth_logs WHERE action = $1 AND created_at >= $2 AND created_at < $3`
	var count int
	if err := r.db.GetContext(ctx, &count, query, action, since, to); err != nil {
		return 0, fmt.Errorf("failed to count logs: %w", err)
	}
	return count, nil
}

func (r *LogRepo) GetUserStats(ctx context.Context, from, to time.Time) ([]models.UserStat, error) {
	query := `WITH mac_user AS (
		SELECT DISTINCT ON (mac) mac, user_id
		FROM auth_logs
		WHERE mac IS NOT NULL AND mac != ''
			AND user_id IS NOT NULL AND user_id != ''
		ORDER BY mac, created_at DESC
	)
	SELECT
		COALESCE(NULLIF(al.user_id, ''), mu.user_id, al.mac, '') as user_id,
		COALESCE(NULLIF(al.mac, ''), '') as mac,
		MAX(al.username) as username,
		COUNT(*) FILTER (WHERE al.action = 'code_generated') as generated,
		COUNT(*) FILTER (WHERE al.action = 'code_used') as logins,
		COUNT(*) FILTER (WHERE al.action = 'login_failed') as failed
		FROM auth_logs al
		LEFT JOIN mac_user mu ON mu.mac = al.mac AND (al.user_id IS NULL OR al.user_id = '')
		WHERE al.created_at >= $1 AND al.created_at < $2
		GROUP BY COALESCE(NULLIF(al.user_id, ''), mu.user_id, al.mac, ''), COALESCE(NULLIF(al.mac, ''), '')
		ORDER BY COALESCE(NULLIF(al.user_id, ''), mu.user_id, al.mac, ''), generated DESC`
	var stats []models.UserStat
	if err := r.db.SelectContext(ctx, &stats, query, from, to); err != nil {
		return nil, fmt.Errorf("failed to get user stats: %w", err)
	}
	return stats, nil
}

func (r *LogRepo) GetLogs(ctx context.Context, from, to time.Time, limit, offset int) ([]models.AuthLog, error) {
	query := `SELECT id, user_id, username, action, code, mac, ip, metadata, created_at
		FROM auth_logs
		WHERE created_at >= $1 AND created_at < $2
		ORDER BY created_at DESC LIMIT $3 OFFSET $4`
	var logs []models.AuthLog
	if err := r.db.SelectContext(ctx, &logs, query, from, to, limit, offset); err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}
	return logs, nil
}

func (r *LogRepo) GetLogsByUsername(ctx context.Context, username string, from, to time.Time, limit, offset int) ([]models.AuthLog, error) {
	query := `SELECT id, user_id, username, action, code, mac, ip, metadata, created_at
		FROM auth_logs
		WHERE username = $1 AND created_at >= $2 AND created_at < $3
		ORDER BY created_at DESC LIMIT $4 OFFSET $5`
	var logs []models.AuthLog
	if err := r.db.SelectContext(ctx, &logs, query, username, from, to, limit, offset); err != nil {
		return nil, fmt.Errorf("failed to get logs by username: %w", err)
	}
	return logs, nil
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
