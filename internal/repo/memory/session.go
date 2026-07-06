package memory

import (
	"context"
	"sync"
	"time"

	"wifi_bot/internal/models"
)

type SessionRepo struct {
	mu       sync.RWMutex
	sessions map[string]*models.WifiSession // code -> session
	codes    map[string]string              // userID -> code
	ttl      time.Duration
}

func NewSessionRepo(ttl time.Duration) *SessionRepo {
	return &SessionRepo{
		sessions: make(map[string]*models.WifiSession),
		codes:    make(map[string]string),
		ttl:      ttl,
	}
}

func (r *SessionRepo) GetByCode(_ context.Context, code string) (*models.WifiSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.sessions[code]
	if !ok {
		return nil, models.ErrSessionNotFound
	}
	if r.ttl > 0 && time.Since(s.CreatedAt) > r.ttl {
		return nil, models.ErrSessionNotFound
	}
	return s, nil
}

func (r *SessionRepo) GetByUser(ctx context.Context, userID string) (*models.WifiSession, error) {
	r.mu.RLock()
	code, ok := r.codes[userID]
	r.mu.RUnlock()
	if !ok {
		return nil, models.ErrSessionNotFound
	}
	return r.GetByCode(ctx, code)
}

func (r *SessionRepo) Create(_ context.Context, session *models.WifiSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.sessions[session.Code] = session
	r.codes[session.UserID] = session.Code
	return nil
}

func (r *SessionRepo) UpdateMac(_ context.Context, code, mac, ip string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.sessions[code]
	if !ok {
		return models.ErrSessionNotFound
	}
	s.Mac = mac
	s.IP = ip
	s.CreatedAt = time.Now()
	return nil
}

func (r *SessionRepo) DeleteByCode(_ context.Context, code string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.sessions[code]
	if !ok {
		return models.ErrSessionNotFound
	}
	delete(r.sessions, code)
	delete(r.codes, s.UserID)
	return nil
}

func (r *SessionRepo) DeleteByUser(_ context.Context, userID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	code, ok := r.codes[userID]
	if !ok {
		return "", models.ErrSessionNotFound
	}
	delete(r.sessions, code)
	delete(r.codes, userID)
	return code, nil
}
