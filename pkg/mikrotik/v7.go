package mikrotik

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type ClientV7 struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

type ipBindingV7 struct {
	ID  string `json:".id"`
	Mac string `json:"mac-address"`
	Type string `json:"type"`
}

type hotspotSessionV7 struct {
	ID         string `json:".id"`
	User       string `json:"user"`
	Mac        string `json:"mac-address"`
	Address    string `json:"address"`
	Uptime     string `json:"uptime"`
	BytesIn    string `json:"bytes-in"`
	BytesOut   string `json:"bytes-out"`
	PacketsIn  string `json:"packets-in"`
	PacketsOut string `json:"packets-out"`
	IdleTime   string `json:"idle-time"`
	Server     string `json:"server"`
}

func newV7(conf *Config) *ClientV7 {
	scheme := "http"
	if conf.UseSSL {
		scheme = "https"
	}
	return &ClientV7{
		baseURL:  fmt.Sprintf("%s://%s:%d/rest", scheme, conf.Host, conf.Port),
		username: conf.Username,
		password: conf.Password,
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: nil, // использует стандартные корневые сертификаты
			},
		},
	}
}

func (c *ClientV7) Disconnect(ctx context.Context, mac string) error {
	sessions, err := c.listActive(ctx)
	if err != nil {
		return fmt.Errorf("failed to list active sessions: %w", err)
	}

	for _, s := range sessions {
		if s.Mac == mac {
			return c.removeSession(ctx, s.ID)
		}
	}

	return fmt.Errorf("session not found for mac %s", mac)
}

func parseInt64(s string) int64 {
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}

func (c *ClientV7) ListSessions(ctx context.Context) ([]HotspotSession, error) {
	raw, err := c.listActive(ctx)
	if err != nil {
		return nil, err
	}
	sessions := make([]HotspotSession, len(raw))
	for i, s := range raw {
		sessions[i] = HotspotSession{
			ID: s.ID, User: s.User, Mac: s.Mac, Address: s.Address,
			Uptime: s.Uptime, IdleTime: s.IdleTime, Server: s.Server,
			BytesIn: parseInt64(s.BytesIn), BytesOut: parseInt64(s.BytesOut),
			PacketsIn: parseInt64(s.PacketsIn), PacketsOut: parseInt64(s.PacketsOut),
		}
	}
	return sessions, nil
}

func (c *ClientV7) listActive(ctx context.Context) ([]hotspotSessionV7, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/ip/hotspot/active", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mikrotik returned status %d", resp.StatusCode)
	}

	var sessions []hotspotSessionV7
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return sessions, nil
}

func (c *ClientV7) removeSession(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("%s/ip/hotspot/active/%s", c.baseURL, url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("mikrotik returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *ClientV7) AddBinding(ctx context.Context, mac string) error {
	bindings, err := c.listBindings(ctx)
	if err != nil {
		return fmt.Errorf("failed to list bindings: %w", err)
	}

	for _, b := range bindings {
		if b.Mac == mac {
			return c.updateBinding(ctx, b.ID, mac)
		}
	}

	return c.createBinding(ctx, mac)
}

func (c *ClientV7) RemoveBinding(ctx context.Context, mac string) error {
	bindings, err := c.listBindings(ctx)
	if err != nil {
		return fmt.Errorf("failed to list bindings: %w", err)
	}

	for _, b := range bindings {
		if b.Mac == mac {
			return c.deleteBinding(ctx, b.ID)
		}
	}

	return nil
}

func (c *ClientV7) listBindings(ctx context.Context) ([]ipBindingV7, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/ip/hotspot/ip-binding", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mikrotik returned status %d", resp.StatusCode)
	}

	var bindings []ipBindingV7
	if err := json.NewDecoder(resp.Body).Decode(&bindings); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return bindings, nil
}

func (c *ClientV7) createBinding(ctx context.Context, mac string) error {
	body := map[string]string{"mac-address": mac, "type": "bypassed"}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", c.baseURL+"/ip/hotspot/ip-binding", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("mikrotik returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *ClientV7) updateBinding(ctx context.Context, id, mac string) error {
	body := map[string]string{"mac-address": mac, "type": "bypassed"}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal body: %w", err)
	}

	endpoint := fmt.Sprintf("%s/ip/hotspot/ip-binding/%s", c.baseURL, url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, "PUT", endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("mikrotik returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *ClientV7) deleteBinding(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("%s/ip/hotspot/ip-binding/%s", c.baseURL, url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("mikrotik returned status %d", resp.StatusCode)
	}
	return nil
}
