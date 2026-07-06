package transport

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"wifi_bot/internal/models"
	"wifi_bot/internal/services"
	"wifi_bot/pkg/logger"
)

func (h *Handler) HandleMattermost(c *gin.Context) {
	var req models.MattermostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Bad Request"})
		return
	}

	if req.Token != h.token {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	logger.Debug("mattermost command",
		logger.StringAttr("user", req.UserID),
		logger.StringAttr("command", req.Command),
	)

	var text string
	bot := h.services.MattermostBot

	switch req.Command {
	case "/wifi":
		if bot != nil && !bot.IsTeamMember(req.UserID) {
			text = "❌ Нет доступа."
			break
		}
		code, err := h.services.Session.GetOrCreateCode(c.Request.Context(), req.UserID, req.UserName)
		if err != nil {
			text = "Ошибка при получении кода. Попробуйте позже."
			logger.Error("failed to get code", logger.ErrAttr(err))
		} else {
			text = "Ваш персональный код для Wi-Fi: **" + code + "**\nВнимание: при вводе на новом устройстве старое будет отключено."
		}

	case "/wifi_reset":
		if bot != nil && !bot.IsTeamMember(req.UserID) {
			text = "❌ Нет доступа."
			break
		}
		if !h.allowRateLimit("wifi:ratelimit:reset:" + req.UserID) {
			text = "Слишком часто. Лимит: 1 раз в 5 минут."
			break
		}
		code, err := h.services.Session.ResetCode(c.Request.Context(), req.UserID, req.UserName)
		if err != nil {
			text = "Ошибка при сбросе кода. Попробуйте позже."
			logger.Error("failed to reset code", logger.ErrAttr(err))
		} else {
			text = "Код сброшен.\nНовый код: **" + code + "**"
		}

	case "/wifi_code":
		if bot == nil || !bot.IsAdmin(req.UserID) {
			text = "❌ Команда только для администраторов."
			break
		}
		name := strings.TrimSpace(req.Text)
		if name == "" {
			text = "Укажите ФИО: `/wifi_code Иванов Иван`"
			break
		}
		if !h.allowRateLimit("wifi:ratelimit:codegen:" + req.UserID) {
			text = "Слишком частые запросы. Подождите."
			break
		}
		code, err := h.services.Session.GetOrCreateCode(c.Request.Context(), "admin_generated:"+name, name)
		if err != nil {
			text = "Ошибка при генерации кода."
			logger.Error("failed to generate admin code", logger.ErrAttr(err))
		} else {
			text = "✅ Код для **" + name + "**: **" + code + "**"
		}

	case "/wifi_stats":
		if bot == nil || !bot.IsAdmin(req.UserID) {
			text = "❌ Команда только для администраторов."
			break
		}
		from, to, label := services.ParseStatsTimeRange(req.Text)

		stats, err := h.services.Stats.Stats(c.Request.Context(), from, to)
		if err != nil {
			text = "Ошибка при получении статистики."
			logger.Error("stats error", logger.ErrAttr(err))
			break
		}

		var bld strings.Builder
		bld.WriteString("📊 **Статистика за " + label + "**\n\n")
		bld.WriteString(fmt.Sprintf("Сгенерировано кодов: **%d**\n", stats.GeneratedToday))
		bld.WriteString(fmt.Sprintf("Использовано кодов: **%d**\n", stats.UsedToday))
		bld.WriteString(fmt.Sprintf("Неудачных попыток: **%d**\n", stats.FailedToday))

		if len(stats.ActiveList) > 0 {
			bld.WriteString(fmt.Sprintf("\nАктивных сессий: **%d**\n", stats.ActiveSessions))
			bld.WriteString("```\n")
			for _, s := range stats.ActiveList {
				bld.WriteString(fmt.Sprintf("%-20s %s\n", s.Username, s.Mac))
			}
			bld.WriteString("```")
		}

		if len(stats.Logs) > 0 {
			bld.WriteString("\nПоследние события:\n```\n")
			for _, l := range stats.Logs {
				bld.WriteString(fmt.Sprintf("%s %-15s %s\n",
					l.CreatedAt.Format("15:04"), l.Action, l.Username))
			}
			bld.WriteString("```")
		}

		text = bld.String()

	default:
		text = "Неизвестная команда. Используйте `/wifi`, `/wifi_reset`, `/wifi_code` или `/wifi_stats`."
	}

	c.JSON(http.StatusOK, models.MattermostResponse{
		ResponseType: "ephemeral",
		Text:         text,
	})
}
