package services

import (
	"context"
	"sync"
	"time"

	"wifi_bot/internal/repo"
	"wifi_bot/pkg/logger"
	mikrotikClient "wifi_bot/pkg/mikrotik"
)

type Collector struct {
	client   mikrotikClient.Client
	hotspot  repo.Hotspot
	interval time.Duration
	mu       sync.Mutex
	running  bool
	stopCh   chan struct{}
}

func NewCollector(client mikrotikClient.Client, hotspot repo.Hotspot, interval time.Duration) *Collector {
	return &Collector{
		client:   client,
		hotspot:  hotspot,
		interval: interval,
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
	sessions, err := c.client.ListSessions(ctx)
	if err != nil {
		logger.Error("collector: failed to list sessions", logger.ErrAttr(err))
		return
	}

	if len(sessions) == 0 {
		return
	}

	if err := c.hotspot.SaveBatch(ctx, sessions); err != nil {
		logger.Error("collector: failed to save sessions", logger.ErrAttr(err))
	}
}
