package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"wifi_bot/internal/models"
	"wifi_bot/internal/repo"
	"wifi_bot/pkg/error_bot"
	"wifi_bot/pkg/logger"
)

type SessionService struct {
	sessionRepo     repo.Session
	logRepo         repo.Log
	userSessionRepo repo.UserSession
	code            *CodeService
	mikrotik        *MikrotikService
	allowReuse      bool
}

type SessionDeps struct {
	SessionRepo     repo.Session
	LogRepo         repo.Log
	UserSessionRepo repo.UserSession
	Code            *CodeService
	Mikrotik        *MikrotikService
	AllowReuse      bool
}

func NewSessionService(deps *SessionDeps) *SessionService {
	return &SessionService{
		sessionRepo:     deps.SessionRepo,
		logRepo:         deps.LogRepo,
		userSessionRepo: deps.UserSessionRepo,
		code:            deps.Code,
		mikrotik:        deps.Mikrotik,
		allowReuse:      deps.AllowReuse,
	}
}

type Session interface {
	GetOrCreateCode(ctx context.Context, userID, username string) (string, error)
	ResetCode(ctx context.Context, userID, username string) (string, error)
	Login(ctx context.Context, code, mac, ip, linkLoginOnly, challenge, linkOrig string) error
}

func (s *SessionService) GetOrCreateCode(ctx context.Context, userID, username string) (string, error) {
	existing, err := s.sessionRepo.GetByUser(ctx, userID)
	if err == nil && existing != nil {
		return existing.Code, nil
	}

	code := s.code.Generate()
	session := &models.WifiSession{
		UserID:    userID,
		Username:  username,
		Code:      code,
		CreatedAt: time.Now(),
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		error_bot.Send(nil, fmt.Sprintf("session: failed to create session: %v", err), nil)
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	if err := s.logRepo.Create(ctx, &models.AuthLog{
		UserID:   userID,
		Username: username,
		Action:   "code_generated",
		Code:     models.StrPtr(code),
	}); err != nil {
		logger.Error("failed to log code_generated", logger.ErrAttr(err))
	}

	return code, nil
}

func (s *SessionService) ResetCode(ctx context.Context, userID, username string) (string, error) {
	mac, err := s.resolveMAC(ctx, userID)
	if err != nil {
		logger.Error("reset code: resolveMAC failed",
			logger.ErrAttr(err), logger.StringAttr("user_id", userID))
	}

	oldCode, err := s.sessionRepo.DeleteByUser(ctx, userID)
	if err == nil {
		if err := s.logRepo.Create(ctx, &models.AuthLog{
			UserID:   userID,
			Username: username,
			Action:   "code_reset",
			Code:     models.StrPtr(oldCode),
		}); err != nil {
			logger.Error("failed to log code_reset", logger.ErrAttr(err))
		}
	} else {
		logger.Error("delete session by user failed", logger.ErrAttr(err), logger.StringAttr("user_id", userID))
	}

	if mac != "" {
		s.mikrotik.Disconnect(ctx, mac)
		if err := s.userSessionRepo.CloseActive(ctx, mac); err != nil {
			logger.Error("close user session failed",
				logger.ErrAttr(err), logger.StringAttr("mac", mac))
		}
	} else {
		logger.Warn("reset code: no mac found for disconnect",
			logger.StringAttr("user_id", userID))
	}

	code := s.code.Generate()
	session := &models.WifiSession{
		UserID:    userID,
		Username:  username,
		Code:      code,
		CreatedAt: time.Now(),
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		error_bot.Send(nil, fmt.Sprintf("session: failed to create session after reset: %v", err), nil)
		return "", fmt.Errorf("failed to create session after reset: %w", err)
	}

	if err := s.logRepo.Create(ctx, &models.AuthLog{
		UserID:   userID,
		Username: username,
		Action:   "code_generated",
		Code:     models.StrPtr(code),
	}); err != nil {
		logger.Error("failed to log code_generated after reset", logger.ErrAttr(err))
	}

	return code, nil
}

func (s *SessionService) resolveMAC(ctx context.Context, userID string) (string, error) {
	mac, err := s.resolveMACFromRedisSession(ctx, userID)
	if mac != "" || err != nil {
		return mac, err
	}

	mac, err = s.resolveMACFromUserSession(ctx, userID)
	if mac != "" || err != nil {
		return mac, err
	}

	mac, err = s.resolveMACFromLastLog(ctx, userID)
	if mac != "" || err != nil {
		return mac, err
	}

	logger.Debug("resolveMAC: no mac found for user",
		logger.StringAttr("user_id", userID))
	return "", nil
}

func (s *SessionService) resolveMACFromRedisSession(ctx context.Context, userID string) (string, error) {
	existing, err := s.sessionRepo.GetByUser(ctx, userID)
	if err != nil || existing == nil {
		logger.Debug("resolveMAC: no redis session",
			logger.StringAttr("user_id", userID),
			logger.ErrAttr(err))
		return "", nil
	}
	if existing.Mac == "" {
		logger.Debug("resolveMAC: redis session has no mac",
			logger.StringAttr("user_id", userID))
		return "", nil
	}
	logger.Debug("resolveMAC: found mac in redis session",
		logger.StringAttr("user_id", userID),
		logger.StringAttr("mac", existing.Mac))
	return existing.Mac, nil
}

func (s *SessionService) resolveMACFromUserSession(ctx context.Context, userID string) (string, error) {
	userSession, err := s.userSessionRepo.GetLastByUserID(ctx, userID)
	if err != nil || userSession == nil {
		logger.Debug("resolveMAC: no user session",
			logger.StringAttr("user_id", userID),
			logger.ErrAttr(err))
		return "", nil
	}
	logger.Debug("resolveMAC: found mac in user_session",
		logger.StringAttr("user_id", userID),
		logger.StringAttr("mac", userSession.Mac),
		logger.BoolAttr("is_active", userSession.IsActive))
	return userSession.Mac, nil
}

func (s *SessionService) resolveMACFromLastLog(ctx context.Context, userID string) (string, error) {
	mac, err := s.logRepo.GetLastMACByUserID(ctx, userID)
	if err != nil {
		logger.Debug("resolveMAC: log lookup error",
			logger.StringAttr("user_id", userID),
			logger.ErrAttr(err))
		return "", nil
	}
	if mac == "" {
		logger.Debug("resolveMAC: no mac in auth_logs",
			logger.StringAttr("user_id", userID))
		return "", nil
	}
	logger.Debug("resolveMAC: found mac in auth_log",
		logger.StringAttr("user_id", userID),
		logger.StringAttr("mac", mac))
	return mac, nil
}

func (s *SessionService) Login(ctx context.Context, code, mac, ip, linkLoginOnly, challenge, linkOrig string) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !s.code.IsValid(code) {
		if err := s.logRepo.Create(ctx, &models.AuthLog{
			Action:   "login_failed",
			Code:     models.StrPtr(code),
			Mac:      models.StrPtr(mac),
			Metadata: models.StrPtr("invalid format"),
		}); err != nil {
			logger.Error("failed to log login_failed invalid format", logger.ErrAttr(err))
		}
		return models.ErrCodeInvalid
	}

	session, err := s.sessionRepo.GetByCode(ctx, code)
	if err != nil {
		if err := s.logRepo.Create(ctx, &models.AuthLog{
			Action:   "login_failed",
			Code:     models.StrPtr(code),
			Mac:      models.StrPtr(mac),
			Metadata: models.StrPtr("code not found or expired"),
		}); err != nil {
			logger.Error("failed to log login_failed not found", logger.ErrAttr(err))
		}
		return fmt.Errorf("invalid code: %w", err)
	}

	if session.Mac != "" && session.Mac != mac {
		if !s.allowReuse {
			if err := s.logRepo.Create(ctx, &models.AuthLog{
				UserID:   session.UserID,
				Action:   "login_failed",
				Code:     models.StrPtr(code),
				Mac:      models.StrPtr(mac),
				Metadata: models.StrPtr("code already used on " + session.Mac),
			}); err != nil {
				logger.Error("failed to log login_failed already used", logger.ErrAttr(err))
			}
			return models.ErrCodeAlreadyUsed
		}
		logger.Info("code taken over by new device",
			logger.StringAttr("old_mac", session.Mac),
			logger.StringAttr("new_mac", mac),
			logger.StringAttr("code", code),
		)
		s.mikrotik.Disconnect(ctx, session.Mac)
		if err := s.userSessionRepo.CloseActive(ctx, session.Mac); err != nil {
			logger.Error("close user session failed",
				logger.ErrAttr(err), logger.StringAttr("mac", session.Mac))
		}
	}

	if err := s.sessionRepo.UpdateMac(ctx, code, mac, ip); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	endpoint := linkLoginOnly
	if endpoint == "" {
		endpoint = "http://192.168.5.250/login"
	}
	if err := s.mikrotik.Auth(ctx, mac, ip, endpoint, code, code, challenge, linkOrig); err != nil {
		if err := s.logRepo.Create(ctx, &models.AuthLog{
			UserID:   session.UserID,
			Username: session.Username,
			Action:   "login_failed",
			Code:     models.StrPtr(code),
			Mac:      models.StrPtr(mac),
			Metadata: models.StrPtr("mikrotik: " + err.Error()),
		}); err != nil {
			logger.Error("failed to log login_failed mikrotik err", logger.ErrAttr(err))
		}
		return fmt.Errorf("%w: %s", models.ErrMikrotikAuth, err.Error())
	}

	if err := s.logRepo.Create(ctx, &models.AuthLog{
		UserID:   session.UserID,
		Username: session.Username,
		Action:   "code_used",
		Code:     models.StrPtr(code),
		Mac:      models.StrPtr(mac),
		IP:       models.StrPtr(ip),
	}); err != nil {
		logger.Error("failed to log code_used", logger.ErrAttr(err))
	}

	if err := s.userSessionRepo.Create(ctx, &models.UserSession{
		UserID:   session.UserID,
		Username: session.Username,
		Code:     code,
		Mac:      mac,
		IP:       ip,
		LoginAt:  time.Now(),
	}); err != nil {
		s.mikrotik.Disconnect(ctx, mac)
		return fmt.Errorf("failed to create user session: %w", err)
	}

	return nil
}
