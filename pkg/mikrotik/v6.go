package mikrotik

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

type ClientV6 struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

type ipBindingV6 struct {
	ID  string `json:".id"`
	Mac string `json:"mac-address"`
	Type string `json:"type"`
}

type hotspotSessionV6 struct {
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

func newV6(conf *Config) *ClientV6 {
	scheme := "http"
	if conf.UseSSL {
		scheme = "https"
	}
	return &ClientV6{
		baseURL:  fmt.Sprintf("%s://%s:%d/rest", scheme, conf.Host, conf.Port),
		username: conf.Username,
		password: conf.Password,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *ClientV6) Disconnect(ctx context.Context, mac string) error {
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

func (c *ClientV6) ListSessions(ctx context.Context) ([]HotspotSession, error) {
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

func (c *ClientV6) listActive(ctx context.Context) ([]hotspotSessionV6, error) {
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

	var sessions []hotspotSessionV6
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return sessions, nil
}

func (c *ClientV6) removeSession(ctx context.Context, id string) error {
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

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mikrotik returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *ClientV6) AddBinding(ctx context.Context, mac string) error {
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

func (c *ClientV6) RemoveBinding(ctx context.Context, mac string) error {
	bindings, err := c.listBindings(ctx)
	if err != nil {
		return fmt.Errorf("failed to list bindings: %w", err)
	}

	if len(bindings) == 0 {
		slog.Debug("mikrotik: no bindings to remove", "mac", mac)
		return nil
	}

	for _, b := range bindings {
		if b.Mac == mac {
			slog.Debug("mikrotik: deleting binding", "id", b.ID, "mac", mac)
			return c.deleteBinding(ctx, b.ID)
		}
	}

	slog.Debug("mikrotik: binding not found for removal", "mac", mac)
	return nil
}

func (c *ClientV6) BlockBinding(ctx context.Context, mac string) error {
	bindings, err := c.listBindings(ctx)
	if err != nil {
		return fmt.Errorf("failed to list bindings: %w", err)
	}

	if len(bindings) == 0 {
		slog.Debug("mikrotik: no bindings to block", "mac", mac)
		return nil
	}

	for _, b := range bindings {
		if b.Mac == mac {
			slog.Debug("mikrotik: blocking binding", "id", b.ID, "mac", mac)
			return c.setBinding(ctx, b.ID, mac, "blocked")
		}
	}

	slog.Debug("mikrotik: binding not found for blocking", "mac", mac)
	return nil
}

type addressListEntryV6 struct {
	ID      string `json:".id"`
	Address string `json:"address"`
	List    string `json:"list"`
	Comment string `json:"comment"`
}

func (c *ClientV6) AddAddressToList(ctx context.Context, ip, list, comment string) error {
	entries, err := c.listAddressList(ctx, "?comment="+url.QueryEscape(comment))
	if err != nil {
		return fmt.Errorf("failed to list address-list: %w", err)
	}

	if len(entries) > 0 {
		id := entries[0].ID
		if entries[0].Address == ip {
			slog.Debug("mikrotik: address-list entry already exists", "id", id, "ip", ip, "comment", comment)
			return nil
		}
		slog.Debug("mikrotik: updating address-list entry", "id", id, "ip", ip, "comment", comment)
		return c.setAddressList(ctx, id, ip, list, comment)
	}

	slog.Debug("mikrotik: adding address-list entry", "ip", ip, "list", list, "comment", comment)
	return c.createAddressList(ctx, ip, list, comment)
}

func (c *ClientV6) RemoveAddressFromList(ctx context.Context, list, comment string) error {
	entries, err := c.listAddressList(ctx, "?list="+url.QueryEscape(list)+"&comment="+url.QueryEscape(comment))
	if err != nil {
		return fmt.Errorf("failed to list address-list: %w", err)
	}

	for _, e := range entries {
		slog.Debug("mikrotik: removing address-list entry", "id", e.ID, "comment", comment)
		if err := c.deleteAddressListEntry(ctx, e.ID); err != nil {
			return fmt.Errorf("failed to remove address-list entry: %w", err)
		}
	}
	return nil
}

func (c *ClientV6) ListAddressList(ctx context.Context, list string) ([]AddressListEntry, error) {
	raw, err := c.listAddressList(ctx, "?list="+url.QueryEscape(list))
	if err != nil {
		return nil, err
	}
	entries := make([]AddressListEntry, len(raw))
	for i, e := range raw {
		entries[i] = AddressListEntry{ID: e.ID, Address: e.Address, List: e.List, Comment: e.Comment}
	}
	return entries, nil
}

func (c *ClientV6) listAddressList(ctx context.Context, query string) ([]addressListEntryV6, error) {
	endpoint := c.baseURL + "/ip/firewall/address-list" + query
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
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

	var entries []addressListEntryV6
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return entries, nil
}

func (c *ClientV6) createAddressList(ctx context.Context, ip, list, comment string) error {
	body := map[string]string{"address": ip, "list": list, "comment": comment}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", c.baseURL+"/ip/firewall/address-list", bytes.NewReader(data))
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

func (c *ClientV6) setAddressList(ctx context.Context, id, ip, list, comment string) error {
	body := map[string]string{"address": ip}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal body: %w", err)
	}

	endpoint := fmt.Sprintf("%s/ip/firewall/address-list/%s", c.baseURL, url.PathEscape(id))
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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mikrotik returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *ClientV6) deleteAddressListEntry(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("%s/ip/firewall/address-list/%s", c.baseURL, url.PathEscape(id))
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

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mikrotik returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *ClientV6) listBindings(ctx context.Context) ([]ipBindingV6, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/ip/hotspot/ip-binding", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")

	slog.Debug("mikrotik: listing bindings", "url", c.baseURL+"/ip/hotspot/ip-binding")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Warn("mikrotik: list bindings error",
			"status", resp.StatusCode,
			"body", string(body),
		)
		return nil, fmt.Errorf("mikrotik returned status %d", resp.StatusCode)
	}

	var bindings []ipBindingV6
	if err := json.NewDecoder(resp.Body).Decode(&bindings); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	slog.Debug("mikrotik: bindings list",
		"count", len(bindings),
		"bindings", fmt.Sprintf("%+v", bindings),
	)
	return bindings, nil
}

func (c *ClientV6) createBinding(ctx context.Context, mac string) error {
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

func (c *ClientV6) updateBinding(ctx context.Context, id, mac string) error {
	return c.setBinding(ctx, id, mac, "bypassed")
}

func (c *ClientV6) setBinding(ctx context.Context, id, mac, bindingType string) error {
	body := map[string]string{"mac-address": mac, "type": bindingType}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal body: %w", err)
	}

	endpoint := fmt.Sprintf("%s/ip/hotspot/ip-binding/%s", c.baseURL, url.PathEscape(id))
	slog.Debug("mikrotik: setting binding",
		"endpoint", endpoint,
		"id", id,
		"mac", mac,
		"type", bindingType,
	)

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

	if resp.StatusCode == http.StatusNotFound {
		slog.Debug("mikrotik: binding not found", "id", id, "mac", mac)
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		slog.Warn("mikrotik: set binding error",
			"id", id,
			"mac", mac,
			"status", resp.StatusCode,
			"body", string(body),
		)
		return fmt.Errorf("mikrotik returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *ClientV6) deleteBinding(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("%s/ip/hotspot/ip-binding/%s", c.baseURL, url.PathEscape(id))
	slog.Debug("mikrotik: deleting binding", "endpoint", endpoint, "id", id)

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

	if resp.StatusCode == http.StatusNotFound {
		slog.Debug("mikrotik: binding already removed", "id", id)
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		slog.Warn("mikrotik: delete binding error",
			"id", id,
			"status", resp.StatusCode,
			"body", string(body),
		)
		return fmt.Errorf("mikrotik returned status %d", resp.StatusCode)
	}
	return nil
}
