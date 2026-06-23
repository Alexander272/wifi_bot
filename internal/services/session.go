package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"wifi_bot/internal/models"
	"wifi_bot/internal/repo"
	"wifi_bot/pkg/logger"
)

type SessionService struct {
	sessionRepo repo.Session
	logRepo     repo.Log
	code        *CodeService
	mikrotik    *MikrotikService
	allowReuse  bool
}

type SessionDeps struct {
	SessionRepo repo.Session
	LogRepo     repo.Log
	Code        *CodeService
	Mikrotik    *MikrotikService
	AllowReuse  bool
}

func NewSessionService(deps *SessionDeps) *SessionService {
	return &SessionService{
		sessionRepo: deps.SessionRepo,
		logRepo:     deps.LogRepo,
		code:        deps.Code,
		mikrotik:    deps.Mikrotik,
		allowReuse:  deps.AllowReuse,
	}
}

type Session interface {
	GetOrCreateCode(ctx context.Context, userID string) (string, error)
	ResetCode(ctx context.Context, userID string) (string, error)
	Login(ctx context.Context, code, mac, ip, linkLoginOnly, challenge, linkOrig string) error
}

func (s *SessionService) GetOrCreateCode(ctx context.Context, userID string) (string, error) {
	existing, err := s.sessionRepo.GetByUser(ctx, userID)
	if err == nil && existing != nil {
		return existing.Code, nil
	}

	code := s.code.Generate()
	session := &models.WifiSession{
		UserID:    userID,
		Code:      code,
		CreatedAt: time.Now(),
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	s.logRepo.Create(ctx, &models.AuthLog{
		UserID: userID,
		Action: "code_generated",
		Code:   code,
	})

	return code, nil
}

func (s *SessionService) ResetCode(ctx context.Context, userID string) (string, error) {
	oldCode, err := s.sessionRepo.DeleteByUser(ctx, userID)
	if err == nil {
		if session, err := s.sessionRepo.GetByCode(ctx, oldCode); err == nil && session.Mac != "" {
			go s.mikrotik.Disconnect(session.Mac)
		}

		s.logRepo.Create(ctx, &models.AuthLog{
			UserID: userID,
			Action: "code_reset",
			Code:   oldCode,
		})
	}

	code := s.code.Generate()
	session := &models.WifiSession{
		UserID:    userID,
		Code:      code,
		CreatedAt: time.Now(),
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return "", fmt.Errorf("failed to create session after reset: %w", err)
	}

	s.logRepo.Create(ctx, &models.AuthLog{
		UserID: userID,
		Action: "code_generated",
		Code:   code,
	})

	return code, nil
}

func (s *SessionService) Login(ctx context.Context, code, mac, ip, linkLoginOnly, challenge, linkOrig string) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !s.code.IsValid(code) {
		return models.ErrCodeInvalid
	}

	session, err := s.sessionRepo.GetByCode(ctx, code)
	if err != nil {
		return fmt.Errorf("invalid code: %w", err)
	}

	if session.Mac != "" && session.Mac != mac {
		if !s.allowReuse {
			return models.ErrCodeAlreadyUsed
		}
		logger.Info("code taken over by new device",
			logger.StringAttr("old_mac", session.Mac),
			logger.StringAttr("new_mac", mac),
			logger.StringAttr("code", code),
		)
		go s.mikrotik.Disconnect(session.Mac)
	}

	if err := s.sessionRepo.UpdateMac(ctx, code, mac, ip); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	endpoint := linkLoginOnly
	if endpoint == "" {
		endpoint = "http://192.168.5.250/login"
	}
	if err := s.mikrotik.Auth(ctx, mac, endpoint, code, code, challenge, linkOrig); err != nil {
		return fmt.Errorf("%w: %s", models.ErrMikrotikAuth, err.Error())
	}

	s.logRepo.Create(ctx, &models.AuthLog{
		UserID: session.UserID,
		Action: "code_used",
		Code:   code,
		Mac:    mac,
		IP:     ip,
	})

	return nil
}
