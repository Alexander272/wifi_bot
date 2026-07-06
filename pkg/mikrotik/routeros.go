package mikrotik

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/go-routeros/routeros/v3"
)

const APIVersionRouterOS = "routeros"

type ClientRouterOS struct {
	host     string
	port     int
	username string
	password string
	useTLS   bool
	mu       sync.Mutex
	conn     *routeros.Client
}

func newRouterOS(conf *Config) *ClientRouterOS {
	return &ClientRouterOS{
		host:     conf.Host,
		port:     conf.Port,
		username: conf.Username,
		password: conf.Password,
		useTLS:   conf.UseSSL,
	}
}

func (c *ClientRouterOS) dial(ctx context.Context) (*routeros.Client, error) {
	addr := net.JoinHostPort(c.host, fmt.Sprintf("%d", c.port))
	slog.Debug("routeros: connecting", "addr", addr)
	if c.useTLS {
		return routeros.DialTLSContext(ctx, addr, c.username, c.password, nil)
	}
	return routeros.DialContext(ctx, addr, c.username, c.password)
}

func (c *ClientRouterOS) getConn(ctx context.Context) (*routeros.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.conn, nil
	}
	cl, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	c.conn = cl
	return cl, nil
}

func (c *ClientRouterOS) invalidateConn() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

func (c *ClientRouterOS) Disconnect(ctx context.Context, mac string) error {
	cl, err := c.getConn(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to routeros: %w", err)
	}

	reply, err := cl.Run("/ip/hotspot/active/print", "?mac-address="+mac)
	if err != nil {
		c.invalidateConn()
		return fmt.Errorf("failed to list active sessions: %w", err)
	}

	for _, re := range reply.Re {
		id := re.Map[".id"]
		slog.Debug("routeros: removing active session", "id", id, "mac", mac)
		if _, err := cl.Run("/ip/hotspot/active/remove", "=.id="+id); err != nil {
			c.invalidateConn()
			return fmt.Errorf("failed to remove session: %w", err)
		}
	}

	if len(reply.Re) == 0 {
		return fmt.Errorf("session not found for mac %s", mac)
	}

	return nil
}

func (c *ClientRouterOS) ListSessions(ctx context.Context) ([]HotspotSession, error) {
	cl, err := c.getConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to routeros: %w", err)
	}

	reply, err := cl.Run("/ip/hotspot/active/print")
	if err != nil {
		c.invalidateConn()
		return nil, fmt.Errorf("failed to list active sessions: %w", err)
	}

	sessions := make([]HotspotSession, len(reply.Re))
	for i, re := range reply.Re {
		sessions[i] = HotspotSession{
			ID:         re.Map[".id"],
			User:       re.Map["user"],
			Mac:        re.Map["mac-address"],
			Address:    re.Map["address"],
			Uptime:     re.Map["uptime"],
			BytesIn:    parseInt64(re.Map["bytes-in"]),
			BytesOut:   parseInt64(re.Map["bytes-out"]),
			PacketsIn:  parseInt64(re.Map["packets-in"]),
			PacketsOut: parseInt64(re.Map["packets-out"]),
			IdleTime:   re.Map["idle-time"],
			Server:     re.Map["server"],
		}
	}
	return sessions, nil
}

func (c *ClientRouterOS) AddBinding(ctx context.Context, mac string) error {
	cl, err := c.getConn(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to routeros: %w", err)
	}

	slog.Debug("routeros: listing bindings", "mac", mac)
	reply, err := cl.Run("/ip/hotspot/ip-binding/print", "?mac-address="+mac)
	if err != nil {
		c.invalidateConn()
		return fmt.Errorf("failed to list bindings: %w", err)
	}

	if len(reply.Re) > 0 {
		id := reply.Re[0].Map[".id"]
		slog.Debug("routeros: updating existing binding", "id", id, "mac", mac)
		_, err = cl.Run("/ip/hotspot/ip-binding/set", "=.id="+id, "=type=bypassed")
		if err != nil {
			c.invalidateConn()
		}
		return err
	}

	slog.Debug("routeros: creating new binding", "mac", mac)
	_, err = cl.Run("/ip/hotspot/ip-binding/add", "=mac-address="+mac, "=type=bypassed")
	if err != nil {
		c.invalidateConn()
	}
	return err
}

func (c *ClientRouterOS) RemoveBinding(ctx context.Context, mac string) error {
	cl, err := c.getConn(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to routeros: %w", err)
	}

	slog.Debug("routeros: listing bindings", "mac", mac)
	reply, err := cl.Run("/ip/hotspot/ip-binding/print", "?mac-address="+mac)
	if err != nil {
		c.invalidateConn()
		return fmt.Errorf("failed to list bindings: %w", err)
	}

	if len(reply.Re) == 0 {
		slog.Debug("routeros: no binding to remove", "mac", mac)
		return nil
	}

	for _, re := range reply.Re {
		id := re.Map[".id"]
		slog.Debug("routeros: removing binding", "id", id, "mac", mac)
		if _, err := cl.Run("/ip/hotspot/ip-binding/remove", "=.id="+id); err != nil {
			c.invalidateConn()
			return fmt.Errorf("failed to remove binding: %w", err)
		}
	}

	return nil
}

func (c *ClientRouterOS) AddAddressToList(ctx context.Context, ip, list, comment string) error {
	cl, err := c.getConn(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to routeros: %w", err)
	}

	slog.Debug("routeros: listing address-list entries", "list", list, "comment", comment)
	reply, err := cl.Run("/ip/firewall/address-list/print", "?comment="+comment)
	if err != nil {
		c.invalidateConn()
		return fmt.Errorf("failed to list address-list: %w", err)
	}

	if len(reply.Re) > 0 {
		id := reply.Re[0].Map[".id"]
		existingIP := reply.Re[0].Map["address"]
		if existingIP == ip {
			slog.Debug("routeros: address-list entry already exists", "id", id, "ip", ip, "comment", comment)
			return nil
		}
		slog.Debug("routeros: updating address-list entry", "id", id, "ip", ip, "comment", comment)
		_, err = cl.Run("/ip/firewall/address-list/set", "=.id="+id, "=address="+ip)
		if err != nil {
			c.invalidateConn()
		}
		return err
	}

	slog.Debug("routeros: adding address-list entry", "ip", ip, "list", list, "comment", comment)
	_, err = cl.Run("/ip/firewall/address-list/add", "=address="+ip, "=list="+list, "=comment="+comment)
	if err != nil {
		c.invalidateConn()
	}
	return err
}

func (c *ClientRouterOS) RemoveAddressFromList(ctx context.Context, list, comment string) error {
	cl, err := c.getConn(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to routeros: %w", err)
	}

	slog.Debug("routeros: listing address-list entries", "list", list, "comment", comment)
	reply, err := cl.Run("/ip/firewall/address-list/print", "?comment="+comment, "?list="+list)
	if err != nil {
		c.invalidateConn()
		return fmt.Errorf("failed to list address-list: %w", err)
	}

	for _, re := range reply.Re {
		id := re.Map[".id"]
		slog.Debug("routeros: removing address-list entry", "id", id, "comment", comment)
		if _, err := cl.Run("/ip/firewall/address-list/remove", "=.id="+id); err != nil {
			c.invalidateConn()
			return fmt.Errorf("failed to remove address-list entry: %w", err)
		}
	}

	return nil
}

func (c *ClientRouterOS) ListAddressList(ctx context.Context, list string) ([]AddressListEntry, error) {
	cl, err := c.getConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to routeros: %w", err)
	}

	reply, err := cl.Run("/ip/firewall/address-list/print", "?list="+list)
	if err != nil {
		c.invalidateConn()
		return nil, fmt.Errorf("failed to list address-list: %w", err)
	}

	entries := make([]AddressListEntry, len(reply.Re))
	for i, re := range reply.Re {
		entries[i] = AddressListEntry{
			ID:      re.Map[".id"],
			Address: re.Map["address"],
			List:    re.Map["list"],
			Comment: re.Map["comment"],
		}
	}
	return entries, nil
}

func (c *ClientRouterOS) BlockBinding(ctx context.Context, mac string) error {
	cl, err := c.getConn(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to routeros: %w", err)
	}

	slog.Debug("routeros: listing bindings", "mac", mac)
	reply, err := cl.Run("/ip/hotspot/ip-binding/print", "?mac-address="+mac)
	if err != nil {
		c.invalidateConn()
		return fmt.Errorf("failed to list bindings: %w", err)
	}

	if len(reply.Re) == 0 {
		slog.Debug("routeros: no binding to block", "mac", mac)
		return nil
	}

	for _, re := range reply.Re {
		id := re.Map[".id"]
		slog.Debug("routeros: blocking binding", "id", id, "mac", mac)
		if _, err := cl.Run("/ip/hotspot/ip-binding/set", "=.id="+id, "=type=blocked"); err != nil {
			c.invalidateConn()
			return fmt.Errorf("failed to block binding: %w", err)
		}
	}

	return nil
}
