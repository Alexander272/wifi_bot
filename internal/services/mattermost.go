package services

import (
	"context"
	"strings"
	"sync"
	"time"

	"wifi_bot/pkg/logger"
	mm "wifi_bot/pkg/mattermost"
)

type MattermostBot struct {
	client    *mm.Client
	ws        *mm.WSClient
	session   Session
	adminIDs  []string
	collector *Collector
}

func NewMattermostBot(client *mm.Client, ws *mm.WSClient, session Session, adminIDs []string, collector *Collector) *MattermostBot {
	b := &MattermostBot{
		client:    client,
		ws:        ws,
		session:   session,
		adminIDs:  adminIDs,
		collector: collector,
	}
	ws.SetHandler(b.handleEvent)
	return b
}

func (b *MattermostBot) Start(ctx context.Context) {
	uid, err := b.client.GetMe()
	if err != nil {
		logger.Error("mattermost: failed to get bot user id", logger.ErrAttr(err))
		return
	}
	b.ws.SetUserID(uid)
	logger.Info("mattermost: bot started", logger.StringAttr("user_id", uid))

	b.ws.Run(ctx)
}

func (b *MattermostBot) handleEvent(ev mm.Event) {
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
	case "wifi", "/wifi", "вайфай", "/вайфай":
		code, err := b.session.GetOrCreateCode(context.Background(), userID)
		if err != nil {
			text = "Ошибка при получении кода. Попробуйте позже."
			logger.Error("mattermost: failed to get code", logger.ErrAttr(err))
		} else {
			text = "Ваш персональный код для Wi-Fi: **" + code + "**\nВнимание: при вводе на новом устройстве старое будет отключено."
		}

	case "wifi_reset", "/wifi_reset", "сбросить", "/сбросить":
		rlKey := "mm:" + userID
		if !checkRateLimit(rlKey) {
			text = "Вы слишком часто сбрасываете код. Лимит: 1 раз в 5 минут."
			break
		}
		code, err := b.session.ResetCode(context.Background(), userID)
		if err != nil {
			text = "Ошибка при сбросе кода. Попробуйте позже."
			logger.Error("mattermost: failed to reset code", logger.ErrAttr(err))
		} else {
			text = "Код сброшен.\nНовый код: **" + code + "**"
		}

	case "wifi_collect", "/wifi_collect", "collect":
		if !b.isAdmin(userID) {
			text = "Команда только для администраторов."
			break
		}
		switch arg {
		case "on", "start":
			b.collector.Start(context.Background())
			text = "Сбор сессий MikroTik запущен."
		case "off", "stop":
			b.collector.Stop()
			text = "Сбор сессий MikroTik остановлен."
		case "status":
			if b.collector.IsRunning() {
				text = "Сбор сессий MikroTik включён."
			} else {
				text = "Сбор сессий MikroTik выключен."
			}
		default:
			text = "Использование:\n" +
				"- `wifi_collect on` — включить сбор\n" +
				"- `wifi_collect off` — выключить сбор\n" +
				"- `wifi_collect status` — статус"
		}

	case "start", "help", "/start", "/help", "помощь", "/помощь":
		text = "Привет! Я бот для генерации кодов доступа к Wi-Fi.\n\n" +
			"Доступные команды:\n" +
			"- `wifi` / `вайфай` — получить код доступа\n" +
			"- `wifi_reset` / `сбросить` — сбросить код и отключить текущее устройство"

	default:
		text = "Неизвестная команда. Напишите `wifi` или `вайфай` чтобы получить код доступа."
	}

	if err := b.client.SendPost(channelID, text); err != nil {
		logger.Error("mattermost: failed to send response", logger.ErrAttr(err))
	}
}

func (b *MattermostBot) isAdmin(userID string) bool {
	for _, id := range b.adminIDs {
		if id == userID {
			return true
		}
	}
	return false
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
	rateMu.Lock()
	defer rateMu.Unlock()
	last, ok := rateLimits[key]
	now := time.Now()
	if ok && now.Sub(last) < 5*time.Minute {
		return false
	}
	rateLimits[key] = now
	return true
}
