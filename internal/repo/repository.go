package repo

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/jmoiron/sqlx"

	"wifi_bot/internal/models"
	"wifi_bot/internal/repo/postgres"
	repoRedis "wifi_bot/internal/repo/redis"
	mikrotikClient "wifi_bot/pkg/mikrotik"
)

type Session interface {
	GetByCode(ctx context.Context, code string) (*models.WifiSession, error)
	GetByUser(ctx context.Context, userID string) (*models.WifiSession, error)
	Create(ctx context.Context, session *models.WifiSession) error
	UpdateMac(ctx context.Context, code, mac, ip string) error
	DeleteByCode(ctx context.Context, code string) error
	DeleteByUser(ctx context.Context, userID string) (string, error)
}

type Log interface {
	Create(ctx context.Context, entry *models.AuthLog) error
	GetByUser(ctx context.Context, userID string, limit, offset int) ([]models.AuthLog, error)
	GetAll(ctx context.Context, limit, offset int) ([]models.AuthLog, error)
}

type Hotspot interface {
	SaveBatch(ctx context.Context, sessions []mikrotikClient.HotspotSession) error
	GetLatest(ctx context.Context, limit int) ([]models.HotspotRecord, error)
}

type Repository struct {
	Session
	Log
	Hotspot
}

func NewRepository(db *sqlx.DB, rdb *redis.Client, codeTTL time.Duration) *Repository {
	return &Repository{
		Session: repoRedis.NewSessionRepo(rdb, codeTTL),
		Log:     postgres.NewLogRepo(db),
		Hotspot: postgres.NewHotspotRepo(db),
	}
}
