package memory

import (
	"context"
	"sync"
	"time"

	"wifi_bot/internal/models"
)

type UserSessionRepo struct {
	mu       sync.RWMutex
	sessions []models.UserSession
	seq      int64
}

func NewUserSessionRepo() *UserSessionRepo {
	return &UserSessionRepo{}
}

func (r *UserSessionRepo) Create(_ context.Context, s *models.UserSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.seq++
	r.sessions = append(r.sessions, models.UserSession{
		ID:          r.seq,
		UserID:      s.UserID,
		Username:    s.Username,
		Code:        s.Code,
		Mac:         s.Mac,
		IP:          s.IP,
		LoginAt:     time.Now(),
		IsActive:    true,
		TTLDuration: s.TTLDuration,
	})
	return nil
}

func (r *UserSessionRepo) CloseActive(_ context.Context, mac string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for i := range r.sessions {
		if r.sessions[i].Mac == mac && r.sessions[i].IsActive {
			r.sessions[i].LogoutAt = &now
			r.sessions[i].IsActive = false
			return nil
		}
	}
	return nil
}

func (r *UserSessionRepo) GetActiveByMAC(_ context.Context, mac string) (*models.UserSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i := range r.sessions {
		if r.sessions[i].Mac == mac && r.sessions[i].IsActive {
			s := r.sessions[i]
			return &s, nil
		}
	}
	return nil, nil
}

func (r *UserSessionRepo) GetActiveByUserID(_ context.Context, userID string) (*models.UserSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i := range r.sessions {
		if r.sessions[i].UserID == userID && r.sessions[i].IsActive {
			s := r.sessions[i]
			return &s, nil
		}
	}
	return nil, nil
}

func (r *UserSessionRepo) GetLastByUserID(_ context.Context, userID string) (*models.UserSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var last *models.UserSession
	for i := range r.sessions {
		if r.sessions[i].UserID == userID {
			s := r.sessions[i]
			if last == nil || s.LoginAt.After(last.LoginAt) {
				last = &s
			}
		}
	}
	return last, nil
}

func (r *UserSessionRepo) ListActive(_ context.Context) ([]models.UserSession, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []models.UserSession
	for i := range r.sessions {
		if r.sessions[i].IsActive {
			result = append(result, r.sessions[i])
		}
	}
	return result, nil
}

func (r *UserSessionRepo) CloseInactiveOlderThan(_ context.Context, t time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	var count int64
	for i := range r.sessions {
		if r.sessions[i].IsActive && r.sessions[i].LoginAt.Before(t) {
			r.sessions[i].LogoutAt = &now
			r.sessions[i].IsActive = false
			count++
		}
	}
	return count, nil
}
