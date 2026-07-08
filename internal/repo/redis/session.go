package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"

	"wifi_bot/internal/models"
)

type SessionRepo struct {
	client *redis.Client
	ttl    time.Duration
}

func NewSessionRepo(client *redis.Client, ttl time.Duration) *SessionRepo {
	return &SessionRepo{client: client, ttl: ttl}
}

func userKey(userID string) string { return fmt.Sprintf("wifi:user:%s:code", userID) }
func codeKey(code string) string   { return fmt.Sprintf("wifi:code:%s:session", code) }

func (r *SessionRepo) GetByCode(ctx context.Context, code string) (*models.WifiSession, error) {
	data, err := r.client.Get(ctx, codeKey(code)).Result()
	if err == redis.Nil {
		return nil, models.ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("redis get error: %w", err)
	}
	var session models.WifiSession
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}
	return &session, nil
}

func (r *SessionRepo) GetByUser(ctx context.Context, userID string) (*models.WifiSession, error) {
	code, err := r.client.Get(ctx, userKey(userID)).Result()
	if err == redis.Nil {
		return nil, models.ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("redis get error: %w", err)
	}
	return r.GetByCode(ctx, code)
}

func (r *SessionRepo) ttlFor(session *models.WifiSession) time.Duration {
	if session.TTLDuration > 0 {
		return session.TTLDuration
	}
	return r.ttl
}

func (r *SessionRepo) Create(ctx context.Context, session *models.WifiSession) error {
	body, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	ttl := r.ttlFor(session)
	pipe := r.client.TxPipeline()
	pipe.Set(ctx, codeKey(session.Code), body, ttl)
	pipe.Set(ctx, userKey(session.UserID), session.Code, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis pipeline error: %w", err)
	}
	return nil
}

func (r *SessionRepo) UpdateMac(ctx context.Context, code, mac, ip string) error {
	session, err := r.GetByCode(ctx, code)
	if err != nil {
		return err
	}
	session.Mac = mac
	session.IP = ip

	body, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	ttl := r.ttlFor(session)
	pipe := r.client.TxPipeline()
	pipe.Set(ctx, codeKey(code), body, ttl)
	pipe.Set(ctx, userKey(session.UserID), session.Code, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis pipeline error: %w", err)
	}
	return nil
}

func (r *SessionRepo) DeleteByCode(ctx context.Context, code string) error {
	session, err := r.GetByCode(ctx, code)
	if err != nil {
		return err
	}

	pipe := r.client.TxPipeline()
	pipe.Del(ctx, codeKey(code))
	pipe.Del(ctx, userKey(session.UserID))
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis pipeline error: %w", err)
	}
	return nil
}

func (r *SessionRepo) DeleteByUser(ctx context.Context, userID string) (string, error) {
	session, err := r.GetByUser(ctx, userID)
	if err != nil {
		return "", err
	}

	pipe := r.client.TxPipeline()
	pipe.Del(ctx, codeKey(session.Code))
	pipe.Del(ctx, userKey(userID))
	if _, err := pipe.Exec(ctx); err != nil {
		return "", fmt.Errorf("redis pipeline error: %w", err)
	}
	return session.Code, nil
}
