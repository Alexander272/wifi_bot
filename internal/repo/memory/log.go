package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"wifi_bot/internal/models"
)

type LogRepo struct {
	mu   sync.RWMutex
	logs []models.AuthLog
}

func NewLogRepo() *LogRepo {
	return &LogRepo{}
}

func (r *LogRepo) Create(_ context.Context, entry *models.AuthLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry.CreatedAt = time.Now()
	r.logs = append(r.logs, *entry)
	return nil
}

func (r *LogRepo) GetByUser(_ context.Context, userID string, limit, offset int) ([]models.AuthLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []models.AuthLog
	for _, l := range r.logs {
		if l.UserID == userID {
			filtered = append(filtered, l)
		}
	}

	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}

	return applyLimitOffset(filtered, limit, offset), nil
}

func (r *LogRepo) GetAll(_ context.Context, limit, offset int) ([]models.AuthLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]models.AuthLog, len(r.logs))
	copy(result, r.logs)
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return applyLimitOffset(result, limit, offset), nil
}

func (r *LogRepo) GetLastMACByUserID(_ context.Context, userID string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var lastMAC string
	var lastTime time.Time
	for _, l := range r.logs {
		if l.UserID == userID && l.Mac != nil && *l.Mac != "" && l.Action == "code_used" {
			if l.CreatedAt.After(lastTime) {
				lastMAC = *l.Mac
				lastTime = l.CreatedAt
			}
		}
	}
	return lastMAC, nil
}

func (r *LogRepo) CountByActionSince(_ context.Context, action string, since, to time.Time) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var count int
	for _, l := range r.logs {
		if l.Action == action && !l.CreatedAt.Before(since) && l.CreatedAt.Before(to) {
			count++
		}
	}
	return count, nil
}

func (r *LogRepo) GetUserStats(_ context.Context, from, to time.Time) ([]models.UserStat, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// build mac → user_id map from all entries
	macUser := make(map[string]string)
	for _, l := range r.logs {
		if l.Mac == nil || *l.Mac == "" || l.UserID == "" {
			continue
		}
		macUser[*l.Mac] = l.UserID
	}

	type groupKey struct {
		id  string
		mac string
	}
	stats := make(map[groupKey]*models.UserStat)
	for _, l := range r.logs {
		if l.CreatedAt.Before(from) || !l.CreatedAt.Before(to) {
			continue
		}
		id := l.UserID
		if id == "" && l.Mac != nil {
			if uid, ok := macUser[*l.Mac]; ok {
				id = uid
			} else {
				id = *l.Mac
			}
		}
		mac := ""
		if l.Mac != nil {
			mac = *l.Mac
		}
		key := groupKey{id: id, mac: mac}
		s, ok := stats[key]
		if !ok {
			s = &models.UserStat{UserID: id, Mac: mac}
			stats[key] = s
		}
		switch l.Action {
		case "code_generated":
			s.Generated++
		case "code_used":
			s.Logins++
		case "login_failed":
			s.Failed++
		}
		if l.Username != "" {
			s.Username = l.Username
		}
	}

	result := make([]models.UserStat, 0, len(stats))
	for _, s := range stats {
		result = append(result, *s)
	}

	// sort by user_id, then generated DESC
	sort.Slice(result, func(i, j int) bool {
		if result[i].UserID != result[j].UserID {
			return result[i].UserID < result[j].UserID
		}
		return result[i].Generated > result[j].Generated
	})

	return result, nil
}

func (r *LogRepo) GetLogs(_ context.Context, from, to time.Time, limit, offset int) ([]models.AuthLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []models.AuthLog
	for _, l := range r.logs {
		if !l.CreatedAt.Before(from) && l.CreatedAt.Before(to) {
			filtered = append(filtered, l)
		}
	}

	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}

	return applyLimitOffset(filtered, limit, offset), nil
}

func (r *LogRepo) GetLogsByUsername(_ context.Context, username string, from, to time.Time, limit, offset int) ([]models.AuthLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []models.AuthLog
	for _, l := range r.logs {
		if l.Username == username && !l.CreatedAt.Before(from) && l.CreatedAt.Before(to) {
			filtered = append(filtered, l)
		}
	}

	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}

	return applyLimitOffset(filtered, limit, offset), nil
}

func applyLimitOffset[T any](items []T, limit, offset int) []T {
	if offset >= len(items) {
		return nil
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}
