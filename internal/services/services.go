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
}

type Deps struct {
	Repo            *repo.Repository
	MikrotikClient  mikrotikClient.Client
	MattermostBot   *MattermostBot
	CollectInterval time.Duration
	AuthTimeout     time.Duration
	MikrotikHost    string
	AuthMethod      string
	AllowReuse      bool
}

func NewServices(deps *Deps) *Services {
	code := NewCodeService()
	mikrotikSvc := NewMikrotikService(deps.MikrotikClient, deps.AuthTimeout, deps.MikrotikHost, deps.AuthMethod)
	session := NewSessionService(&SessionDeps{
		SessionRepo: deps.Repo.Session,
		LogRepo:     deps.Repo.Log,
		Code:        code,
		Mikrotik:    mikrotikSvc,
		AllowReuse:  deps.AllowReuse,
	})
	collector := NewCollector(deps.MikrotikClient, deps.Repo.Hotspot, deps.CollectInterval)

	return &Services{
		Session: session, MattermostBot: deps.MattermostBot,
		Collector: collector,
	}
}
