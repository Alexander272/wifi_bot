package postgres

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"wifi_bot/internal/models"
	mikrotikClient "wifi_bot/pkg/mikrotik"
)

type HotspotRepo struct {
	db *sqlx.DB
}

func NewHotspotRepo(db *sqlx.DB) *HotspotRepo {
	return &HotspotRepo{db: db}
}

func (r *HotspotRepo) SaveBatch(ctx context.Context, sessions []mikrotikClient.HotspotSession) error {
	if len(sessions) == 0 {
		return nil
	}

	query := `INSERT INTO hotspot_sessions (username, mac, address, uptime, bytes_in, bytes_out, packets_in, packets_out, idle_time, server)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, s := range sessions {
		if _, err := tx.ExecContext(ctx, query,
			s.User, s.Mac, s.Address, s.Uptime,
			s.BytesIn, s.BytesOut, s.PacketsIn, s.PacketsOut,
			s.IdleTime, s.Server,
		); err != nil {
			return fmt.Errorf("insert session: %w", err)
		}
	}

	return tx.Commit()
}

func (r *HotspotRepo) GetLatest(ctx context.Context, limit int) ([]models.HotspotRecord, error) {
	query := `SELECT id, username, mac, address, uptime, bytes_in, bytes_out, packets_in, packets_out, idle_time, server, collected_at
		FROM hotspot_sessions ORDER BY collected_at DESC LIMIT $1`
	var records []models.HotspotRecord
	if err := r.db.SelectContext(ctx, &records, query, limit); err != nil {
		return nil, fmt.Errorf("get hotspot records: %w", err)
	}
	return records, nil
}
