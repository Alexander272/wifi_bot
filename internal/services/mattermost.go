package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"wifi_bot/internal/models"
	"wifi_bot/pkg/error_bot"
	"wifi_bot/pkg/logger"
	mm "wifi_bot/pkg/mattermost"
)

type userNameEntry struct {
	name string
	exp  time.Time
}

type MattermostBot struct {
	client        *mm.Client
	ws            *mm.WSClient
	session       Session
	stats         *StatsService
	adminIDs      []string
	collector     *Collector
	teamName      string
	teamID        string
	teamMembers   map[string]struct{}
	teamAdmins    map[string]struct{}
	memberMu      sync.RWMutex
	memberStopCh  chan struct{}
	userNameCache map[string]userNameEntry
	userNameMu    sync.RWMutex
}

func NewMattermostBot(client *mm.Client, ws *mm.WSClient, session Session, stats *StatsService, adminIDs []string, collector *Collector, teamName string) *MattermostBot {
	b := &MattermostBot{
		client:        client,
		ws:            ws,
		session:       session,
		stats:         stats,
		adminIDs:      adminIDs,
		collector:     collector,
		teamName:      teamName,
		userNameCache: make(map[string]userNameEntry),
	}
	ws.SetHandler(b.handleEvent)
	return b
}

func (b *MattermostBot) userName(userID, senderName string) string {
	if senderName != "" {
		b.userNameMu.Lock()
		b.userNameCache[userID] = userNameEntry{name: senderName, exp: time.Now().Add(time.Hour)}
		b.userNameMu.Unlock()
		return senderName
	}

	b.userNameMu.RLock()
	entry, ok := b.userNameCache[userID]
	b.userNameMu.RUnlock()
	if ok && time.Now().Before(entry.exp) {
		return entry.name
	}

	user, err := b.client.GetUser(userID)
	if err != nil {
		logger.Debug("mattermost: failed to get user name", logger.ErrAttr(err))
		return ""
	}

	b.userNameMu.Lock()
	b.userNameCache[userID] = userNameEntry{name: user.Username, exp: time.Now().Add(time.Hour)}
	b.userNameMu.Unlock()
	return user.Username
}

func (b *MattermostBot) Start(ctx context.Context) {
	uid, err := b.client.GetMe()
	if err != nil {
		logger.Error("mattermost: failed to get bot user id", logger.ErrAttr(err))
		error_bot.Send(nil, fmt.Sprintf("mattermost: failed to get bot user id: %v", err), nil)
		return
	}
	b.ws.SetUserID(uid)
	logger.Info("mattermost: bot started", logger.StringAttr("user_id", uid))

	if b.teamName != "" {
		team, err := b.client.GetTeamByName(b.teamName)
		if err != nil {
			logger.Error("mattermost: failed to resolve team",
				logger.ErrAttr(err), logger.StringAttr("team", b.teamName))
		} else {
			b.teamID = team.ID
			logger.Info("mattermost: team resolved",
				logger.StringAttr("team", b.teamName),
				logger.StringAttr("team_id", b.teamID))
			b.refreshMembers()
			b.startMemberCache(ctx)
		}
	}

	b.ws.Run(ctx)
}

func (b *MattermostBot) startMemberCache(ctx context.Context) {
	b.memberStopCh = make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("member cache: panic recovered",
					logger.StringAttr("panic", fmt.Sprintf("%v", r)))
				error_bot.Send(nil, fmt.Sprintf("member cache: panic recovered: %v", r), nil)
			}
		}()

		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				b.refreshMembers()
			case <-b.memberStopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (b *MattermostBot) refreshMembers() {
	users, err := b.client.GetActiveTeamUsers(b.teamID)
	if err != nil {
		logger.Error("mattermost: failed to refresh team members", logger.ErrAttr(err))
		return
	}

	m := make(map[string]struct{}, len(users))
	a := make(map[string]struct{}, len(users))

	for _, u := range users {
		m[u.ID] = struct{}{}
		if strings.Contains(u.Roles, "team_admin") || strings.Contains(u.Roles, "system_admin") {
			a[u.ID] = struct{}{}
		}
	}

	b.memberMu.Lock()
	b.teamMembers = m
	b.teamAdmins = a
	b.memberMu.Unlock()

	logger.Debug("mattermost: team members refreshed",
		logger.IntAttr("total", len(m)),
		logger.IntAttr("admins", len(a)))
}

func (b *MattermostBot) isTeamMember(userID string) bool {
	if b.teamID == "" {
		return true
	}
	b.memberMu.RLock()
	defer b.memberMu.RUnlock()
	_, ok := b.teamMembers[userID]
	return ok
}

func (b *MattermostBot) IsTeamMember(userID string) bool {
	return b.isTeamMember(userID)
}

func (b *MattermostBot) IsAdmin(userID string) bool {
	return b.isAdmin(userID)
}

func (b *MattermostBot) isAdmin(userID string) bool {
	for _, id := range b.adminIDs {
		if id == userID {
			return true
		}
	}
	if b.teamID == "" {
		return false
	}
	b.memberMu.RLock()
	defer b.memberMu.RUnlock()
	_, ok := b.teamAdmins[userID]
	return ok
}

func (b *MattermostBot) handleEvent(ctx context.Context, ev mm.Event) {
	message := strings.TrimSpace(ev.Post.Message)
	channelID := ev.Post.ChannelID
	userID := ev.Post.UserID

	logger.Debug("mattermost: dm received",
		logger.StringAttr("user_id", userID),
		logger.StringAttr("message", message),
	)

	command, arg := parseCommand(message)

	var text string
	switch command {
	case "wifi", "wi-fi", "/wifi", "вайфай", "вай-фай", "/вайфай":
		text = b.handleWifi(ctx, userID, ev.Username)
	case "reset", "/reset", "сбросить", "/сбросить":
		if b.isAdmin(userID) && len(strings.Fields(message)) > 1 {
			text = b.handleAdminReset(ctx, userID, message)
		} else {
			text = b.handleReset(ctx, userID, ev.Username)
		}
	case "wifi_code", "/wifi_code", "code", "/code", "код", "сгенерить":
		text = b.handleAdminCode(ctx, userID, message)
	case "stats", "/stats", "статистика", "/статистика":
		text = b.handleStats(ctx, userID, message)
	case "userstats", "/userstats", "юзерстат", "пользователь":
		text = b.handleUserStats(ctx, userID, message)
	case "wifi_collect", "/wifi_collect", "collect":
		text = b.handleCollect(ctx, userID, arg)
	case "start", "help", "man", "/start", "/help", "помощь", "/помощь":
		text = b.handleHelp(userID)
	default:
		text = "Неизвестная команда. Напишите `wifi` или `вайфай` чтобы получить код доступа."
	}

	if err := b.client.SendPost(channelID, text); err != nil {
		logger.Error("mattermost: failed to send response", logger.ErrAttr(err))
	}
}

func (b *MattermostBot) handleWifi(ctx context.Context, userID, username string) string {
	if !b.isTeamMember(userID) {
		return "❌ Нет доступа."
	}
	code, err := b.session.GetOrCreateCode(ctx, userID, b.userName(userID, username))
	if err != nil {
		logger.Error("mattermost: failed to get code", logger.ErrAttr(err))
		return "Ошибка при получении кода. Попробуйте позже."
	}
	return "Ваш персональный код для Wi-Fi: **" + code + "**\nВнимание: при вводе на новом устройстве старое будет отключено."
}

func (b *MattermostBot) handleReset(ctx context.Context, userID, username string) string {
	if !b.isTeamMember(userID) {
		return "❌ Нет доступа."
	}
	rlKey := "wifi:ratelimit:reset:" + userID
	if !checkRateLimit(rlKey) {
		return "Вы слишком часто сбрасываете код. Лимит: 1 раз в 5 минут."
	}
	code, err := b.session.ResetCode(ctx, userID, b.userName(userID, username))
	if err != nil {
		logger.Error("mattermost: failed to reset code", logger.ErrAttr(err))
		return "Ошибка при сбросе кода. Попробуйте позже."
	}
	return "Код сброшен.\nНовый код: **" + code + "**"
}

func (b *MattermostBot) handleAdminCode(ctx context.Context, userID, message string) string {
	if !b.isAdmin(userID) {
		return "❌ Команда только для администраторов."
	}
	parts := strings.Fields(message)
	if len(parts) < 2 {
		return "Укажите ФИО: `code Иванов Иван [TTL]`\nTTL — опционально, например: `72h`, `48h`, `30m`"
	}

	var ttl time.Duration
	nameParts := parts[1:]
	if last := nameParts[len(nameParts)-1]; len(nameParts) > 1 {
		if d, err := time.ParseDuration(last); err == nil {
			ttl = d
			nameParts = nameParts[:len(nameParts)-1]
		}
	}
	name := strings.Join(nameParts, " ")

	rlKey := "wifi:ratelimit:codegen:" + userID
	if !checkRateLimitDuration(rlKey, 30*time.Second) {
		return "Слишком частые запросы. Подождите 30 секунд."
	}

	var code string
	var err error
	if ttl > 0 {
		code, err = b.session.GetOrCreateCodeWithTTL(ctx, "admin_generated:"+name, name, ttl)
	} else {
		code, err = b.session.GetOrCreateCode(ctx, "admin_generated:"+name, name)
	}
	if err != nil {
		logger.Error("mattermost: failed to generate admin code", logger.ErrAttr(err))
		return "Ошибка при генерации кода."
	}
	resp := "✅ Код для **" + name + "**: **" + code + "**"
	if ttl > 0 {
		resp += "\n⏱ Время жизни: **" + ttl.String() + "**"
	}
	return resp
}

func (b *MattermostBot) handleAdminReset(ctx context.Context, userID, message string) string {
	if !b.isAdmin(userID) {
		return "❌ Команда только для администраторов."
	}
	parts := strings.Fields(message)
	if len(parts) < 2 {
		return "Укажите ФИО: `reset Иванов Иван [TTL]`"
	}

	var ttl time.Duration
	nameParts := parts[1:]
	if last := nameParts[len(nameParts)-1]; len(nameParts) > 1 {
		if d, err := time.ParseDuration(last); err == nil {
			ttl = d
			nameParts = nameParts[:len(nameParts)-1]
		}
	}
	name := strings.Join(nameParts, " ")

	rlKey := "wifi:ratelimit:codegen:" + userID
	if !checkRateLimitDuration(rlKey, 30*time.Second) {
		return "Слишком частые запросы. Подождите 30 секунд."
	}

	code, err := b.session.ResetCodeWithTTL(ctx, "admin_generated:"+name, name, ttl)
	if err != nil {
		logger.Error("mattermost: failed to admin reset code", logger.ErrAttr(err))
		return "Ошибка при сбросе кода."
	}
	resp := "✅ Код для **" + name + "** сброшен.\nНовый код: **" + code + "**"
	if ttl > 0 {
		resp += "\n⏱ Время жизни: **" + ttl.String() + "**"
	}
	return resp
}

func (b *MattermostBot) handleStats(ctx context.Context, userID, message string) string {
	if !b.isAdmin(userID) {
		return "❌ Команда только для администраторов."
	}
	parts := strings.Fields(message)
	dateArgs := ""
	if len(parts) > 1 {
		dateArgs = strings.Join(parts[1:], " ")
	}
	from, to, label := ParseStatsTimeRange(dateArgs)

	stats, err := b.stats.Stats(ctx, from, to)
	if err != nil {
		logger.Error("mattermost: stats error", logger.ErrAttr(err))
		return "Ошибка при получении статистики."
	}

	var bld strings.Builder
	bld.WriteString("📊 **Статистика за " + label + "**\n\n")
	bld.WriteString(fmt.Sprintf("Сгенерировано кодов: **%d**\n", stats.GeneratedToday))
	bld.WriteString(fmt.Sprintf("Использовано кодов: **%d**\n", stats.UsedToday))
	bld.WriteString(fmt.Sprintf("Неудачных попыток: **%d**\n", stats.FailedToday))

	hasActive := len(stats.ActiveList) > 0
	hasUserStats := false
	for _, u := range stats.UserStats {
		if u.Generated > 0 || u.Logins > 0 || u.Failed > 0 {
			hasUserStats = true
			break
		}
	}
	if hasActive || hasUserStats {
		bld.WriteString(fmt.Sprintf("\nАктивных сессий: **%d**\n", stats.ActiveSessions))
	}

	if hasUserStats {
		merged := mergeEmptyMACRows(stats.UserStats)

		bld.WriteString("\n👤 **По пользователям:**\n\n")
		bld.WriteString("| Пользователь | MAC | Ген | Исп | Ош |\n")
		bld.WriteString("|:---|---:|---:|---:|---:|\n")
		var prevUserID string
		for _, u := range merged {
			if u.Generated == 0 && u.Logins == 0 && u.Failed == 0 {
				continue
			}
			name := u.Username
			if name == "" {
				name = u.UserID
			}
			if u.UserID == prevUserID {
				name = ""
			} else {
				prevUserID = u.UserID
			}
			if name == "" && u.Mac == "" {
				name = "❓ Неизвестно"
			}
			bld.WriteString(fmt.Sprintf("| %s | %s | %d | %d | %d |\n", name, u.Mac, u.Generated, u.Logins, u.Failed))
		}
	}

	if len(stats.Logs) > 0 {
		bld.WriteString("\nПоследние события:\n```\n")
		for _, l := range stats.Logs {
			bld.WriteString(fmt.Sprintf("%s %-15s %s\n",
				l.CreatedAt.Format("15:04"), l.Action, l.Username))
		}
		bld.WriteString("```")
	}

	return bld.String()
}

func (b *MattermostBot) handleUserStats(ctx context.Context, userID, message string) string {
	if !b.isAdmin(userID) {
		return "❌ Команда только для администраторов."
	}

	parts := strings.Fields(message)
	if len(parts) < 2 {
		return "Укажите пользователя: `userstats Иванов Иван`\n" +
			"Можно указать период: `userstats Иванов Иван 2026-06-01 2026-06-15`"
	}

	dateStart := len(parts)
	for i := 1; i < len(parts); i++ {
		if _, ok := parseDate(parts[i]); ok {
			dateStart = i
			break
		}
	}

	targetUsername := strings.Join(parts[1:dateStart], " ")
	dateArgs := ""
	if dateStart < len(parts) {
		dateArgs = strings.Join(parts[dateStart:], " ")
	}
	from, to, label := ParseStatsTimeRange(dateArgs)

	detail, err := b.stats.UserStats(ctx, targetUsername, from, to)
	if err != nil {
		logger.Error("mattermost: user stats error", logger.ErrAttr(err))
		return "Ошибка при получении статистики пользователя."
	}

	var bld strings.Builder
	bld.WriteString("📊 **Статистика пользователя: " + targetUsername + "**")
	bld.WriteString("\nЗа период: " + label)
	bld.WriteString(fmt.Sprintf("\n\nСгенерировано кодов: **%d**", detail.Generated))
	bld.WriteString(fmt.Sprintf("\nИспользовано кодов: **%d**", detail.Logins))
	bld.WriteString(fmt.Sprintf("\nНеудачных попыток: **%d**", detail.Failed))
	if detail.LastMAC != "" {
		bld.WriteString(fmt.Sprintf("\nПоследний MAC: **%s**", detail.LastMAC))
	}

	if len(detail.Logs) > 0 {
		bld.WriteString("\n\n📋 **Действия:**\n```\n")
		for _, l := range detail.Logs {
			mac := ""
			if l.Mac != nil {
				mac = *l.Mac
			}
			meta := ""
			if l.Metadata != nil {
				meta = *l.Metadata
			}
			bld.WriteString(fmt.Sprintf("%s %-14s %-20s %s\n",
				l.CreatedAt.Format("02.01 15:04"), l.Action, mac, meta))
		}
		bld.WriteString("```")
	}

	return bld.String()
}

func (b *MattermostBot) handleCollect(ctx context.Context, userID, arg string) string {
	if !b.isAdmin(userID) {
		return "Команда только для администраторов."
	}
	switch arg {
	case "on", "start":
		b.collector.Start(ctx)
		return "Сбор сессий MikroTik запущен."
	case "off", "stop":
		b.collector.Stop()
		return "Сбор сессий MikroTik остановлен."
	case "status":
		if b.collector.IsRunning() {
			return "Сбор сессий MikroTik включён."
		}
		return "Сбор сессий MikroTik выключен."
	default:
		return "Использование:\n" +
			"- `wifi_collect on` — включить сбор\n" +
			"- `wifi_collect off` — выключить сбор\n" +
			"- `wifi_collect status` — статус"
	}
}

func (b *MattermostBot) handleHelp(userID string) string {
	text := "Привет! Я бот для генерации кодов доступа к Wi-Fi.\n\n" +
		"Доступные команды:\n" +
		"- `wifi` / `вайфай` — получить код доступа\n" +
		"- `reset` / `сбросить` — сбросить код и отключить текущее устройство"
	if b.isAdmin(userID) {
		text += "\n\nАдмин-команды:\n" +
			"- `code <ФИО> [TTL]` / `сгенерить <ФИО>` — сгенерировать код\n" +
			"  TTL: `72h`, `48h`, `30m` и т.д. (по умолчанию из конфига)\n" +
			"- `reset <ФИО> [TTL]` / `сбросить <ФИО> [TTL]` — сбросить код пользователя\n" +
			"- `stats [дата]` / `статистика [дата]` — статистика использования\n" +
			"- `userstats <ФИО> [дата]` / `юзерстат <ФИО> [дата]` — статистика пользователя"
	}
	return text
}

func mergeEmptyMACRows(stats []models.UserStat) []models.UserStat {
	firstNonEmpty := make(map[string]int)
	for i, u := range stats {
		if u.Mac != "" {
			if _, ok := firstNonEmpty[u.UserID]; !ok {
				firstNonEmpty[u.UserID] = i
			}
		}
	}

	for i := range stats {
		u := &stats[i]
		if u.Mac == "" {
			if ni, ok := firstNonEmpty[u.UserID]; ok {
				stats[ni].Generated += u.Generated
				stats[ni].Logins += u.Logins
				stats[ni].Failed += u.Failed
				u.Generated = -1
			}
		}
	}

	merged := make([]models.UserStat, 0, len(stats))
	for _, u := range stats {
		if u.Generated >= 0 {
			merged = append(merged, u)
		}
	}
	return merged
}

func parseCommand(msg string) (string, string) {
	parts := strings.Fields(msg)
	if len(parts) == 0 {
		return "", ""
	}
	cmd := strings.ToLower(strings.TrimSpace(parts[0]))
	arg := ""
	if len(parts) > 1 {
		arg = strings.ToLower(strings.TrimSpace(parts[1]))
	}
	return cmd, arg
}

var rateMu sync.Mutex
var rateLimits = make(map[string]time.Time)

func checkRateLimit(key string) bool {
	return checkRateLimitDuration(key, 5*time.Minute)
}

func checkRateLimitDuration(key string, dur time.Duration) bool {
	rateMu.Lock()
	defer rateMu.Unlock()
	last, ok := rateLimits[key]
	now := time.Now()
	if ok && now.Sub(last) < dur {
		return false
	}
	rateLimits[key] = now
	return true
}
