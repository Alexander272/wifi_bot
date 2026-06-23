package mikrotik

import (
	"context"
	"fmt"
)

type HotspotSession struct {
	ID         string
	User       string
	Mac        string
	Address    string
	Uptime     string
	BytesIn    int64
	BytesOut   int64
	PacketsIn  int64
	PacketsOut int64
	IdleTime   string
	Server     string
}

type Client interface {
	Disconnect(ctx context.Context, mac string) error
	ListSessions(ctx context.Context) ([]HotspotSession, error)
	AddBinding(ctx context.Context, mac string) error
	RemoveBinding(ctx context.Context, mac string) error
}

type Config struct {
	APIVersion string
	Host       string
	Port       int
	Username   string
	Password   string
	UseSSL     bool
}

func NewClient(conf *Config) (Client, error) {
	switch conf.APIVersion {
	case "v6":
		return newV6(conf), nil
	case "v7":
		return newV7(conf), nil
	default:
		return nil, fmt.Errorf("unknown mikrotik api version: %s", conf.APIVersion)
	}
}
