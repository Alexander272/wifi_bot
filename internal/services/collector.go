package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"wifi_bot/internal/models"
	"wifi_bot/internal/repo"
	"wifi_bot/pkg/error_bot"
	"wifi_bot/pkg/logger"
	mikrotikClient "wifi_bot/pkg/mikrotik"
)

type Collector struct {
	client      mikrotikClient.Client
	mikrotik    *MikrotikService
	hotspot     repo.Hotspot
	userSession repo.UserSession
	interval    time.Duration
	codeTTL     time.Duration
	mu          sync.Mutex
	running     bool
	stopCh      chan struct{}
	cycleCount      int
	lastMikrotikErr bool
	lastDBErr       bool
}

func NewCollector(client mikrotikClient.Client, mikrotik *MikrotikService, hotspot repo.Hotspot, userSession repo.UserSession, interval, codeTTL time.Duration) *Collector {
	return &Collector{
		client:      client,
		mikrotik:    mikrotik,
		hotspot:     hotspot,
		userSession: userSession,
		interval:    interval,
		codeTTL:     codeTTL,
	}
}

func (c *Collector) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

func (c *Collector) Start(ctx context.Context) {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.stopCh = make(chan struct{})
	c.mu.Unlock()

	logger.Info("collector: started", logger.StringAttr("interval", c.interval.String()))

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("collector: panic recovered",
					logger.StringAttr("panic", fmt.Sprintf("%v", r)))
				error_bot.Send(nil, fmt.Sprintf("collector: panic recovered: %v", r), nil)
			}
		}()

		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		c.collect(ctx)

		for {
			select {
			case <-ticker.C:
				c.collect(ctx)
			case <-c.stopCh:
				logger.Info("collector: stopped")
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (c *Collector) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running {
		return
	}
	c.running = false
	close(c.stopCh)
}

func (c *Collector) Toggle(ctx context.Context, on bool) {
	if on {
		c.Start(ctx)
	} else {
		c.Stop()
	}
}

func (c *Collector) collect(ctx context.Context) {
	c.cycleCount++

	mikrotikSessions, err := c.client.ListSessions(ctx)
	if err != nil {
		logger.Error("collector: failed to list sessions", logger.ErrAttr(err))
		if !c.lastMikrotikErr {
			c.lastMikrotikErr = true
			error_bot.Send(nil, fmt.Sprintf("collector: mikrotik unavailable: %v", err), nil)
		}
		return
	}
	c.lastMikrotikErr = false

	if len(mikrotikSessions) > 0 {
		if err := c.hotspot.SaveBatch(ctx, mikrotikSessions); err != nil {
			logger.Error("collector: failed to save sessions", logger.ErrAttr(err))
		}
	}

	active, err := c.userSession.ListActive(ctx)
	if err != nil {
		logger.Error("collector: failed to list active user sessions", logger.ErrAttr(err))
		if !c.lastDBErr {
			c.lastDBErr = true
			error_bot.Send(nil, fmt.Sprintf("collector: database unavailable: %v", err), nil)
		}
		return
	}
	c.lastDBErr = false

	c.syncUserSessions(ctx, mikrotikSessions, active)
	if err := c.mikrotik.SyncAddressList(ctx, active); err != nil {
		logger.Error("collector: sync address list failed", logger.ErrAttr(err))
	}

	if c.cycleCount%10 == 0 {
		c.cleanupExpiredSessions(ctx)
	}
}

func (c *Collector) ttlFor(s models.UserSession) time.Duration {
	if s.TTLDuration > 0 {
		return s.TTLDuration
	}
	return c.codeTTL
}

func (c *Collector) cleanupExpiredSessions(ctx context.Context) {
	if c.codeTTL <= 0 {
		return
	}

	now := time.Now()
	globalThreshold := now.Add(-c.codeTTL - time.Hour)

	active, err := c.userSession.ListActive(ctx)
	if err != nil {
		logger.Error("safety-net: failed to list active sessions", logger.ErrAttr(err))
		return
	}

	var earliestClose time.Time
	for _, s := range active {
		ttl := c.ttlFor(s)
		threshold := now.Add(-ttl - time.Hour)
		if s.LoginAt.Before(threshold) {
			logger.Info("safety-net: disconnecting expired session",
				logger.StringAttr("mac", s.Mac),
				logger.StringAttr("user_id", s.UserID),
				logger.StringAttr("username", s.Username),
				logger.TimeAttr("login_at", s.LoginAt),
			)
			c.mikrotik.Disconnect(ctx, s.Mac)
		}
		if s.LoginAt.Before(globalThreshold) && ttl == c.codeTTL {
			if earliestClose.IsZero() || s.LoginAt.Before(earliestClose) {
				earliestClose = s.LoginAt
			}
		}
	}

	if !earliestClose.IsZero() {
		n, err := c.userSession.CloseInactiveOlderThan(ctx, earliestClose)
		if err != nil {
			logger.Error("safety-net: failed to close expired sessions", logger.ErrAttr(err))
			return
		}
		if n > 0 {
			logger.Info("safety-net: closed expired sessions", logger.IntAttr("count", int(n)))
		}
	}
}

func (c *Collector) syncUserSessions(ctx context.Context, mikrotikSessions []mikrotikClient.HotspotSession, active []models.UserSession) {
	if len(active) == 0 {
		return
	}

	activeMACs := make(map[string]struct{}, len(mikrotikSessions))
	for _, s := range mikrotikSessions {
		activeMACs[s.Mac] = struct{}{}
	}

	now := time.Now()

	for _, s := range active {
		if len(mikrotikSessions) > 0 {
			if _, ok := activeMACs[s.Mac]; !ok {
				logger.Info("collector: session not in mikrotik, cleaning up",
					logger.StringAttr("mac", s.Mac),
					logger.StringAttr("user_id", s.UserID),
					logger.StringAttr("username", s.Username),
				)
				c.mikrotik.Disconnect(ctx, s.Mac)
				if err := c.userSession.CloseActive(ctx, s.Mac); err != nil {
					logger.Error("collector: failed to close stale user session", logger.ErrAttr(err))
				}
				continue
			}
		}
		ttl := c.ttlFor(s)
		if ttl > 0 && s.LoginAt.Add(ttl).Before(now) {
			logger.Info("collector: code ttl expired, disconnecting",
				logger.StringAttr("mac", s.Mac),
				logger.StringAttr("user_id", s.UserID),
				logger.StringAttr("username", s.Username),
			)
			c.mikrotik.Disconnect(ctx, s.Mac)
			if err := c.userSession.CloseActive(ctx, s.Mac); err != nil {
				logger.Error("collector: failed to close expired user session", logger.ErrAttr(err))
			}
		}
	}
}
