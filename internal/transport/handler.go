package transport

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"wifi_bot/internal/services"
	"wifi_bot/web"
)

type Handler struct {
	services     *services.Services
	token        string
	deepLink     string
	templates    *template.Template
	rateLimiters sync.Map
}

func NewHandler(svc *services.Services, token, server, teamName, botUsername string) *Handler {
	host := strings.TrimPrefix(server, "https://")
	host = strings.TrimPrefix(host, "http://")
	deepLink := fmt.Sprintf("mattermost://%s/%s/messages/@%s", host, teamName, botUsername)

	return &Handler{
		services: svc,
		token:    token,
		deepLink: deepLink,
		templates: template.Must(
			template.New("").Funcs(template.FuncMap{
				"safeURL": func(s string) template.URL { return template.URL(s) },
			}).ParseFS(web.Templates, "templates/*.html"),
		),
	}
}

func (h *Handler) Init() *gin.Engine {
	router := gin.New()
	router.SetHTMLTemplate(h.templates)
	router.Use(gin.Logger(), gin.Recovery(), securityHeaders(), minifyMiddleware())

	router.POST("/api/mattermost", h.HandleMattermost)
	router.GET("/login", h.HandlePortalPage)
	router.GET("/portal", h.HandlePortalPage)
	router.POST("/portal/login", h.HandlePortalLogin)
	router.GET("/portal/success", h.HandlePortalSuccess)
	router.StaticFileFS("/favicon.ico", "templates/favicon.ico", http.FS(web.Templates))
	router.StaticFileFS("/static/logo.webp", "templates/logo.webp", http.FS(web.Templates))

	return router
}

func (h *Handler) allowRateLimit(key string) bool {
	l, _ := h.rateLimiters.LoadOrStore(key, rate.NewLimiter(rate.Every(5*time.Minute), 2))
	return l.(*rate.Limiter).Allow()
}

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob:; font-src 'self'; connect-src 'self' ws: wss:; "+
				"frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		c.Header("Permissions-Policy",
			"camera=(), microphone=(), geolocation=(), gyroscope=(), "+
				"accelerometer=(), magnetometer=(), usb=(), payment=(), "+
				"display-capture=(), document-domain=()")
		c.Next()
	}
}
