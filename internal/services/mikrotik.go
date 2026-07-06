package services

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"wifi_bot/internal/models"
	"wifi_bot/pkg/logger"
	mikrotikClient "wifi_bot/pkg/mikrotik"
)

type MikrotikService struct {
	client      mikrotikClient.Client
	authTimeout time.Duration
	host        string
	authMethod  string
	addressList string
}

func NewMikrotikService(client mikrotikClient.Client, authTimeout time.Duration, host, authMethod, addressList string) *MikrotikService {
	return &MikrotikService{
		client:      client,
		authTimeout: authTimeout,
		host:        host,
		authMethod:  authMethod,
		addressList: addressList,
	}
}

func (s *MikrotikService) Auth(ctx context.Context, mac, ip, endpoint, username, password, challenge, linkOrig string) error {
	switch s.authMethod {
	case "mac_binding":
		logger.Debug("adding mac binding", logger.StringAttr("mac", mac))
		if err := s.client.AddBinding(ctx, mac); err != nil {
			return err
		}
		if err := s.addToAddressList(ctx, mac, ip); err != nil {
			logger.Error("auth: failed to add address-list entry",
				logger.ErrAttr(err), logger.StringAttr("mac", mac))
		}
		return nil
	}

	logger.Debug("mikrotik auth request",
		logger.StringAttr("endpoint", endpoint),
		logger.StringAttr("username", username),
	)

	dst := linkOrig
	if dst == "" {
		dst = "google.com"
	}

	useCHAP := challenge != ""
	if useCHAP {
		if _, err := hex.DecodeString(challenge); err != nil {
			logger.Warn("challenge is not valid hex, falling back to password auth",
				logger.StringAttr("challenge", challenge),
			)
			useCHAP = false
		}
	}

	data := url.Values{}
	data.Set("username", username)
	data.Set("dst", dst)

	if useCHAP {
		chapID := []byte{0}
		challengeBytes, _ := hex.DecodeString(challenge)
		h := md5.New()
		h.Write(chapID)
		h.Write([]byte(password))
		h.Write(challengeBytes)
		response := hex.EncodeToString(h.Sum(nil))

		data.Set("response", response)
		data.Set("chap-id", "00")
		data.Set("challenge", challenge)

		logger.Debug("auth request (CHAP)",
			logger.StringAttr("endpoint", endpoint),
			logger.StringAttr("response", response),
		)
	} else {
		data.Set("password", password)

		logger.Debug("auth request (PAP)", logger.StringAttr("endpoint", endpoint))
	}

	logger.Debug("auth data", logger.StringAttr("username", username), logger.StringAttr("dst", dst))

	authCtx, cancel := context.WithTimeout(ctx, s.authTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(authCtx, "POST", endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read auth response: %w", err)
	}
	logger.Debug("mikrotik response",
		logger.IntAttr("status", resp.StatusCode),
		logger.StringAttr("body", string(body)),
	)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mikrotik auth returned status %d (endpoint=%s)", resp.StatusCode, endpoint)
	}

	if err := s.addToAddressList(ctx, mac, ip); err != nil {
		logger.Error("auth: failed to add address-list entry",
			logger.ErrAttr(err), logger.StringAttr("mac", mac))
	}
	return nil
}

func (s *MikrotikService) addToAddressList(ctx context.Context, mac, ip string) error {
	if s.addressList == "" {
		return nil
	}
	comment := "portal-" + mac
	logger.Debug("adding to address list",
		logger.StringAttr("mac", mac),
		logger.StringAttr("ip", ip),
		logger.StringAttr("list", s.addressList),
	)
	if err := s.client.AddAddressToList(ctx, ip, s.addressList, comment); err != nil {
		logger.Error("failed to add address-list entry",
			logger.ErrAttr(err), logger.StringAttr("mac", mac))
		return err
	}
	return nil
}

func (s *MikrotikService) removeFromAddressList(ctx context.Context, mac string) error {
	if s.addressList == "" {
		return nil
	}
	comment := "portal-" + mac
	logger.Debug("removing from address list",
		logger.StringAttr("mac", mac),
		logger.StringAttr("list", s.addressList),
	)
	if err := s.client.RemoveAddressFromList(ctx, s.addressList, comment); err != nil {
		logger.Error("failed to remove address-list entry",
			logger.ErrAttr(err), logger.StringAttr("mac", mac))
		return err
	}
	return nil
}

func (s *MikrotikService) SyncAddressList(ctx context.Context, activeSessions []models.UserSession) error {
	if s.addressList == "" {
		return nil
	}

	entries, err := s.client.ListAddressList(ctx, s.addressList)
	if err != nil {
		return fmt.Errorf("failed to list address-list: %w", err)
	}

	entryByMAC := make(map[string]mikrotikClient.AddressListEntry, len(entries))
	for _, e := range entries {
		if len(e.Comment) > 7 && e.Comment[:7] == "portal-" {
			entryByMAC[e.Comment[7:]] = e
		}
	}

	for _, session := range activeSessions {
		comment := "portal-" + session.Mac
		existing, ok := entryByMAC[session.Mac]
		if ok {
			if existing.Address == session.IP {
				delete(entryByMAC, session.Mac)
				continue
			}
			logger.Debug("address-list entry IP mismatch, updating",
				logger.StringAttr("mac", session.Mac),
				logger.StringAttr("old_ip", existing.Address),
				logger.StringAttr("new_ip", session.IP),
			)
			if err := s.client.RemoveAddressFromList(ctx, s.addressList, comment); err != nil {
				logger.Error("failed to remove stale address-list entry",
					logger.ErrAttr(err), logger.StringAttr("mac", session.Mac))
			}
			delete(entryByMAC, session.Mac)
		}
		logger.Debug("adding missing address-list entry",
			logger.StringAttr("mac", session.Mac),
			logger.StringAttr("ip", session.IP),
		)
		if err := s.client.AddAddressToList(ctx, session.IP, s.addressList, comment); err != nil {
			logger.Error("failed to add address-list entry",
				logger.ErrAttr(err), logger.StringAttr("mac", session.Mac))
		}
	}

	for mac, entry := range entryByMAC {
		logger.Info("collector: removing orphaned address-list entry",
			logger.StringAttr("mac", mac),
			logger.StringAttr("ip", entry.Address),
		)
		if err := s.client.RemoveAddressFromList(ctx, s.addressList, "portal-"+mac); err != nil {
			logger.Error("failed to remove orphaned address-list entry",
				logger.ErrAttr(err), logger.StringAttr("mac", mac))
		}
	}
	return nil
}

func (s *MikrotikService) HealthCheck(ctx context.Context) error {
	checkURL := fmt.Sprintf("http://%s/login", s.host)
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(checkCtx, "GET", checkURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("mikrotik hotspot unreachable: %w", err)
	}
	resp.Body.Close()
	return nil
}

func (s *MikrotikService) Disconnect(ctx context.Context, mac string) {
	logger.Info("disconnecting mikrotik session", logger.StringAttr("mac", mac))

	if err := s.removeFromAddressList(ctx, mac); err != nil {
		logger.Error("disconnect: failed to remove address-list entry",
			logger.ErrAttr(err), logger.StringAttr("mac", mac))
	}

	switch s.authMethod {
	case "mac_binding":
		if err := s.client.RemoveBinding(ctx, mac); err != nil {
			logger.Error("remove binding failed, trying to block instead",
				logger.ErrAttr(err), logger.StringAttr("mac", mac))
			if err := s.client.BlockBinding(ctx, mac); err != nil {
				logger.Error("block binding also failed",
					logger.ErrAttr(err), logger.StringAttr("mac", mac))
			}
		}
		if err := s.client.Disconnect(ctx, mac); err != nil {
			logger.Debug("no active hotspot session for mac (expected)", logger.StringAttr("mac", mac))
		}
	default:
		if err := s.client.Disconnect(ctx, mac); err != nil {
			logger.Error("mikrotik disconnect failed", logger.ErrAttr(err), logger.StringAttr("mac", mac))
		}
	}
}
