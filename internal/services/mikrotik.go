package services

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"wifi_bot/pkg/logger"
	mikrotikClient "wifi_bot/pkg/mikrotik"
)

type MikrotikService struct {
	client      mikrotikClient.Client
	authTimeout time.Duration
	host        string
	authMethod  string
}

func NewMikrotikService(client mikrotikClient.Client, authTimeout time.Duration, host, authMethod string) *MikrotikService {
	return &MikrotikService{
		client:      client,
		authTimeout: authTimeout,
		host:        host,
		authMethod:  authMethod,
	}
}

func (s *MikrotikService) Auth(ctx context.Context, mac, endpoint, username, password, challenge, linkOrig string) error {
	switch s.authMethod {
	case "mac_binding":
		logger.Debug("adding mac binding", logger.StringAttr("mac", mac))
		return s.client.AddBinding(ctx, mac)
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

		log.Printf("Auth request to %s (CHAP)", endpoint)
		log.Printf("Response: %s", response)
	} else {
		data.Set("password", password)

		log.Printf("Auth request to %s (PAP)", endpoint)
	}

	log.Printf("Data: username=%s, dst=%s", username, dst)

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

	body, _ := io.ReadAll(resp.Body)
	log.Printf("Mikrotik response status: %d", resp.StatusCode)
	log.Printf("Mikrotik response body: %s", string(body))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mikrotik auth returned status %d (endpoint=%s)", resp.StatusCode, endpoint)
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

func (s *MikrotikService) Disconnect(mac string) {
	logger.Info("disconnecting mikrotik session", logger.StringAttr("mac", mac))
	if err := s.client.Disconnect(context.Background(), mac); err != nil {
		logger.Error("mikrotik disconnect failed", logger.ErrAttr(err), logger.StringAttr("mac", mac))
	}
	if s.authMethod == "mac_binding" {
		if err := s.client.RemoveBinding(context.Background(), mac); err != nil {
			logger.Error("remove binding failed", logger.ErrAttr(err), logger.StringAttr("mac", mac))
		}
	}
}
