package transport

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"wifi_bot/internal/models"
	"wifi_bot/pkg/error_bot"
	"wifi_bot/pkg/logger"
)

type portalData struct {
	Mac           string
	IP            string
	LinkLoginOnly string
	Challenge     string
	LinkOrig      string
	Error         string
	MMLink        string
	HTTPSLink     string
}

type successData struct {
	Mac string
	IP  string
}

func (h *Handler) HandlePortalPage(c *gin.Context) {
	logger.Debug("portal page request", logger.StringAttr("uri", c.Request.RequestURI))

	mac := c.Query("mac")
	ip := c.Query("ip")
	linkLoginOnly := c.Query("link-login-only")
	challenge := c.Query("challenge")
	linkOrig := c.Query("link-orig")

	if mac == "" || linkLoginOnly == "" {
		c.String(http.StatusBadRequest, "Доступ напрямую запрещен. Перейдите через Wi-Fi авторизацию.")
		return
	}

	c.HTML(http.StatusOK, "portal.html", portalData{
		Mac: mac, IP: ip, LinkLoginOnly: linkLoginOnly, Challenge: challenge,
		LinkOrig: linkOrig, MMLink: h.mmLink, HTTPSLink: h.httpsLink,
	})
}

func (h *Handler) HandlePortalLogin(c *gin.Context) {
	code := c.PostForm("code")
	mac := c.PostForm("mac")
	ip := c.PostForm("ip")
	linkLoginOnly := c.PostForm("link_login_only")
	challenge := c.PostForm("challenge")

	if code == "" || mac == "" || linkLoginOnly == "" {
		c.HTML(http.StatusOK, "portal.html", portalData{
			Mac: mac, IP: ip, LinkLoginOnly: linkLoginOnly, Challenge: challenge,
			Error:     "Заполните все поля.",
			MMLink:    h.mmLink,
			HTTPSLink: h.httpsLink,
		})
		return
	}

	linkOrig := c.PostForm("link_orig")

	err := h.services.Session.Login(c.Request.Context(), code, mac, ip, linkLoginOnly, challenge, linkOrig)
	if err != nil {
		logger.Error("login failed", logger.ErrAttr(err), logger.StringAttr("mac", mac))

		errMsg := "Неверный или просроченный код. Запросите новый в Mattermost (wifi)."
		switch {
		case errors.Is(err, models.ErrCodeInvalid):
			errMsg = "Неверный формат кода. Код состоит из 6 символов (цифры и буквы)."
		case errors.Is(err, models.ErrCodeAlreadyUsed):
			errMsg = "Код уже используется на другом устройстве. Сбросьте код в Mattermost (/wifi_reset)."
		case errors.Is(err, models.ErrMikrotikAuth):
			errMsg = "Временные проблемы на сервере, попробуйте позже."
			error_bot.Send(c, err.Error(), nil)
		}

		c.HTML(http.StatusOK, "portal.html", portalData{
			Mac: mac, IP: ip, LinkLoginOnly: linkLoginOnly, Challenge: challenge,
			LinkOrig: linkOrig, Error: errMsg,
			MMLink:    h.mmLink,
			HTTPSLink: h.httpsLink,
		})
		return
	}

	c.Redirect(http.StatusFound, "/portal/success?mac="+mac+"&ip="+ip)
}

func (h *Handler) HandlePortalSuccess(c *gin.Context) {
	mac := c.Query("mac")
	ip := c.Query("ip")
	c.HTML(http.StatusOK, "success.html", successData{Mac: mac, IP: ip})
}
