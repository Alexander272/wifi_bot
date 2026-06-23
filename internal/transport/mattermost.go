package transport

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"wifi_bot/internal/models"
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

	switch req.Command {
	case "/wifi":
		code, err := h.services.Session.GetOrCreateCode(context.Background(), req.UserID)
		if err != nil {
			text = "Ошибка при получении кода. Попробуйте позже."
			logger.Error("failed to get code", logger.ErrAttr(err))
		} else {
			text = "Ваш персональный код для Wi-Fi: **" + code + "**"
			// text = "Ваш персональный код для Wi-Fi: **" + code + "**\nВнимание: при вводе на новом устройстве старое будет отключено."
		}

	case "/wifi_reset":
		if !h.allowRateLimit(req.UserID) {
			text = "Вы слишком часто сбрасываете код. Лимит: 1 раз в 5 минут."
			break
		}
		code, err := h.services.Session.ResetCode(context.Background(), req.UserID)
		if err != nil {
			text = "Ошибка при сбросе кода. Попробуйте позже."
			logger.Error("failed to reset code", logger.ErrAttr(err))
		} else {
			text = "Код сброшен.\nНовый код: **" + code + "**"
		}

	default:
		text = "Неизвестная команда. Используйте `/wifi` или `/wifi_reset`."
	}

	c.JSON(http.StatusOK, models.MattermostResponse{
		ResponseType: "ephemeral",
		Text:         text,
	})
}
