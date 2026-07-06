package memory

import (
	"context"
	"sync"
	"time"

	"wifi_bot/internal/models"
	mikrotikClient "wifi_bot/pkg/mikrotik"
)

type HotspotRepo struct {
	mu      sync.RWMutex
	records []models.HotspotRecord
	seq     int64
}

func NewHotspotRepo() *HotspotRepo {
	return &HotspotRepo{}
}

func (r *HotspotRepo) SaveBatch(_ context.Context, sessions []mikrotikClient.HotspotSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for _, s := range sessions {
		r.seq++
		r.records = append(r.records, models.HotspotRecord{
			ID:          r.seq,
			Username:    s.User,
			Mac:         s.Mac,
			Address:     s.Address,
			Uptime:      s.Uptime,
			BytesIn:     s.BytesIn,
			BytesOut:    s.BytesOut,
			PacketsIn:   s.PacketsIn,
			PacketsOut:  s.PacketsOut,
			IdleTime:    s.IdleTime,
			Server:      s.Server,
			CollectedAt: now,
		})
	}
	return nil
}

func (r *HotspotRepo) GetLatest(_ context.Context, limit int) ([]models.HotspotRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.records) == 0 {
		return nil, nil
	}

	start := len(r.records) - limit
	if start < 0 {
		start = 0
	}

	result := make([]models.HotspotRecord, len(r.records)-start)
	copy(result, r.records[start:])

	// newest first
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return result, nil
}
