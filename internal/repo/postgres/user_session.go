package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"wifi_bot/internal/models"
)

type UserSessionRepo struct {
	db *sqlx.DB
}

func NewUserSessionRepo(db *sqlx.DB) *UserSessionRepo {
	return &UserSessionRepo{db: db}
}

func (r *UserSessionRepo) Create(ctx context.Context, s *models.UserSession) error {
	query := `INSERT INTO user_sessions (user_id, username, code, mac, ip, login_at, is_active, ttl_duration)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := r.db.ExecContext(ctx, query, s.UserID, s.Username, s.Code, s.Mac, s.IP, s.LoginAt, true, int64(s.TTLDuration))
	if err != nil {
		return fmt.Errorf("failed to create user session: %w", err)
	}
	return nil
}

func (r *UserSessionRepo) CloseActive(ctx context.Context, mac string) error {
	query := `UPDATE user_sessions SET logout_at = NOW(), is_active = false
		WHERE mac = $1 AND is_active = true`
	_, err := r.db.ExecContext(ctx, query, mac)
	if err != nil {
		return fmt.Errorf("failed to close user session: %w", err)
	}
	return nil
}

func (r *UserSessionRepo) GetActiveByMAC(ctx context.Context, mac string) (*models.UserSession, error) {
	query := `SELECT id, user_id, username, code, mac, ip, login_at, logout_at, is_active, ttl_duration
		FROM user_sessions WHERE mac = $1 AND is_active = true LIMIT 1`
	var s models.UserSession
	if err := r.db.GetContext(ctx, &s, query, mac); err != nil {
		return nil, fmt.Errorf("failed to get active session: %w", err)
	}
	return &s, nil
}

func (r *UserSessionRepo) GetActiveByUserID(ctx context.Context, userID string) (*models.UserSession, error) {
	query := `SELECT id, user_id, username, code, mac, ip, login_at, logout_at, is_active, ttl_duration
		FROM user_sessions WHERE user_id = $1 AND is_active = true ORDER BY login_at DESC LIMIT 1`
	var s models.UserSession
	if err := r.db.GetContext(ctx, &s, query, userID); err != nil {
		return nil, fmt.Errorf("failed to get active session by user: %w", err)
	}
	return &s, nil
}

func (r *UserSessionRepo) GetLastByUserID(ctx context.Context, userID string) (*models.UserSession, error) {
	query := `SELECT id, user_id, username, code, mac, ip, login_at, logout_at, is_active, ttl_duration
		FROM user_sessions WHERE user_id = $1 ORDER BY login_at DESC LIMIT 1`
	var s models.UserSession
	if err := r.db.GetContext(ctx, &s, query, userID); err != nil {
		return nil, fmt.Errorf("failed to get last session by user: %w", err)
	}
	return &s, nil
}

func (r *UserSessionRepo) ListActive(ctx context.Context) ([]models.UserSession, error) {
	query := `SELECT id, user_id, username, code, mac, ip, login_at, logout_at, is_active, ttl_duration
		FROM user_sessions WHERE is_active = true ORDER BY login_at DESC`
	var sessions []models.UserSession
	if err := r.db.SelectContext(ctx, &sessions, query); err != nil {
		return nil, fmt.Errorf("failed to list active sessions: %w", err)
	}
	return sessions, nil
}

func (r *UserSessionRepo) CloseInactiveOlderThan(ctx context.Context, t time.Time) (int64, error) {
	query := `UPDATE user_sessions SET logout_at = NOW(), is_active = false
		WHERE is_active = true AND login_at < $1`
	res, err := r.db.ExecContext(ctx, query, t)
	if err != nil {
		return 0, fmt.Errorf("failed to close inactive sessions: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}
	return n, nil
}
