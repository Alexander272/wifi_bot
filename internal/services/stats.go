package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"wifi_bot/internal/models"
	"wifi_bot/internal/repo"
)

func parseDate(s string) (time.Time, bool) {
	formats := []string{"2006-01-02", "02.01.2006", "02.01.06"}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func ParseStatsTimeRange(arg string) (from, to time.Time, label string) {
	now := time.Now()
	from = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	to = now
	label = "сегодня"

	parts := strings.Fields(arg)
	if len(parts) >= 1 {
		// "01.07.2026 - 06.07.2026" (пробелы вокруг дефиса)
		if len(parts) >= 3 && parts[1] == "-" {
			if t1, ok1 := parseDate(parts[0]); ok1 {
				if t2, ok2 := parseDate(parts[2]); ok2 {
					from = t1
					to = t2.Add(24 * time.Hour)
					label = from.Format("02.01.2006") + " — " + t2.Format("02.01.2006")
					return
				}
			}
		}
		// "01.07.2026-06.07.2026" (дефис без пробелов)
		if len(parts) == 1 && strings.Contains(parts[0], "-") {
			if dates := strings.SplitN(parts[0], "-", 2); len(dates) == 2 {
				if t1, ok1 := parseDate(dates[0]); ok1 {
					if t2, ok2 := parseDate(dates[1]); ok2 {
						from = t1
						to = t2.Add(24 * time.Hour)
						label = from.Format("02.01.2006") + " — " + t2.Format("02.01.2006")
						return
					}
				}
			}
		}
	}
	if len(parts) >= 1 && parts[0] != "" {
		if t, ok := parseDate(parts[0]); ok {
			from = t
			to = t.Add(24 * time.Hour)
			label = t.Format("02.01.2006")
		}
	}
	if len(parts) >= 2 {
		if t, ok := parseDate(parts[1]); ok {
			to = t.Add(24 * time.Hour)
			label = from.Format("02.01.2006") + " — " + t.Format("02.01.2006")
		}
	}
	return
}

type Stats struct {
	ActiveSessions int
	ActiveList     []models.UserSession
	UserStats      []models.UserStat
	Logs           []models.AuthLog
	GeneratedToday int
	UsedToday      int
	FailedToday    int
}

type StatsService struct {
	logRepo         repo.Log
	userSessionRepo repo.UserSession
}

func NewStatsService(logRepo repo.Log, userSessionRepo repo.UserSession) *StatsService {
	return &StatsService{
		logRepo:         logRepo,
		userSessionRepo: userSessionRepo,
	}
}

type UserDetailStats struct {
	Username  string
	Generated int
	Logins    int
	Failed    int
	LastMAC   string
	Logs      []models.AuthLog
}

func (s *StatsService) UserStats(ctx context.Context, username string, from, to time.Time) (*UserDetailStats, error) {
	logs, err := s.logRepo.GetLogsByUsername(ctx, username, from, to, 1000, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get user logs: %w", err)
	}

	detail := &UserDetailStats{
		Username: username,
		Logs:     logs,
	}

	for _, l := range logs {
		switch l.Action {
		case "code_generated":
			detail.Generated++
		case "code_used":
			detail.Logins++
			if l.Mac != nil && *l.Mac != "" {
				detail.LastMAC = *l.Mac
			}
		case "login_failed":
			detail.Failed++
		}
	}

	return detail, nil
}

func (s *StatsService) Stats(ctx context.Context, from, to time.Time) (*Stats, error) {
	active, err := s.userSessionRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list active sessions: %w", err)
	}

	generated, err := s.logRepo.CountByActionSince(ctx, "code_generated", from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to count generated: %w", err)
	}

	used, err := s.logRepo.CountByActionSince(ctx, "code_used", from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to count used: %w", err)
	}

	failed, err := s.logRepo.CountByActionSince(ctx, "login_failed", from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to count failed: %w", err)
	}

	userStats, err := s.logRepo.GetUserStats(ctx, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to get user stats: %w", err)
	}

	logs, err := s.logRepo.GetLogs(ctx, from, to, 10, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}

	return &Stats{
		ActiveSessions: len(active),
		ActiveList:     active,
		UserStats:      userStats,
		Logs:           logs,
		GeneratedToday: generated,
		UsedToday:      used,
		FailedToday:    failed,
	}, nil
}
