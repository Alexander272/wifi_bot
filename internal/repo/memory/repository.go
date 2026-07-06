package memory

import (
	"time"

	"wifi_bot/internal/repo"
)

func NewRepository(codeTTL time.Duration) *repo.Repository {
	return &repo.Repository{
		Session:     NewSessionRepo(codeTTL),
		Log:         NewLogRepo(),
		Hotspot:     NewHotspotRepo(),
		UserSession: NewUserSessionRepo(),
	}
}
