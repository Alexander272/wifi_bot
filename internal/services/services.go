package services

import (
	"time"

	"wifi_bot/internal/repo"
	mikrotikClient "wifi_bot/pkg/mikrotik"
)

type Services struct {
	Session
	MattermostBot *MattermostBot
	Collector     *Collector
	Stats         *StatsService
}

type Deps struct {
	Repo            *repo.Repository
	MikrotikClient  mikrotikClient.Client
	MattermostBot   *MattermostBot
	CollectInterval time.Duration
	CodeTTL         time.Duration
	AuthTimeout     time.Duration
	MikrotikHost    string
	AuthMethod      string
	AllowReuse      bool
	AddressList     string
}

func NewServices(deps *Deps) *Services {
	code := NewCodeService()
	mikrotikSvc := NewMikrotikService(deps.MikrotikClient, deps.AuthTimeout, deps.MikrotikHost, deps.AuthMethod, deps.AddressList)
	session := NewSessionService(&SessionDeps{
		SessionRepo:     deps.Repo.Session,
		LogRepo:         deps.Repo.Log,
		UserSessionRepo: deps.Repo.UserSession,
		Code:            code,
		Mikrotik:        mikrotikSvc,
		AllowReuse:      deps.AllowReuse,
	})
	collector := NewCollector(deps.MikrotikClient, mikrotikSvc,
		deps.Repo.Hotspot, deps.Repo.UserSession,
		deps.CollectInterval, deps.CodeTTL)
	stats := NewStatsService(deps.Repo.Log, deps.Repo.UserSession)

	return &Services{
		Session: session, MattermostBot: deps.MattermostBot,
		Collector: collector, Stats: stats,
	}
}
