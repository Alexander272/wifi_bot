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
	GetLastMACByUserID(ctx context.Context, userID string) (string, error)
	CountByActionSince(ctx context.Context, action string, since, to time.Time) (int, error)
	GetUserStats(ctx context.Context, from, to time.Time) ([]models.UserStat, error)
	GetLogs(ctx context.Context, from, to time.Time, limit, offset int) ([]models.AuthLog, error)
	GetLogsByUsername(ctx context.Context, username string, from, to time.Time, limit, offset int) ([]models.AuthLog, error)
}

type Hotspot interface {
	SaveBatch(ctx context.Context, sessions []mikrotikClient.HotspotSession) error
	GetLatest(ctx context.Context, limit int) ([]models.HotspotRecord, error)
}

type UserSession interface {
	Create(ctx context.Context, s *models.UserSession) error
	CloseActive(ctx context.Context, mac string) error
	GetActiveByMAC(ctx context.Context, mac string) (*models.UserSession, error)
	GetActiveByUserID(ctx context.Context, userID string) (*models.UserSession, error)
	GetLastByUserID(ctx context.Context, userID string) (*models.UserSession, error)
	ListActive(ctx context.Context) ([]models.UserSession, error)
	CloseInactiveOlderThan(ctx context.Context, t time.Time) (int64, error)
}

type Repository struct {
	Session
	Log
	Hotspot
	UserSession
}

func NewRepository(db *sqlx.DB, rdb *redis.Client, codeTTL time.Duration) *Repository {
	return &Repository{
		Session:     repoRedis.NewSessionRepo(rdb, codeTTL),
		Log:         postgres.NewLogRepo(db),
		Hotspot:     postgres.NewHotspotRepo(db),
		UserSession: postgres.NewUserSessionRepo(db),
	}
}
