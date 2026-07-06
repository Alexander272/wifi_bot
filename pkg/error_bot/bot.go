package error_bot

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"

	"wifi_bot/pkg/logger"
)

type Message struct {
	Service *Service     `json:"service"`
	Data    *MessageData `json:"data"`
}

type Service struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type MessageData struct {
	Date    string `json:"date"`
	Error   string `json:"error"`
	IP      string `json:"ip"`
	URL     string `json:"url"`
	Request string `json:"request,omitempty"`
}

func Send(c *gin.Context, e string, request any) {
	data := &MessageData{
		Date:  time.Now().Format("02/01/2006 - 15:04:05"),
		Error: e,
	}
	if c != nil {
		data.IP = c.ClientIP()
		data.URL = fmt.Sprintf("%s %s", c.Request.Method, c.Request.URL.String())
	}

	if request != nil {
		req, err := json.MarshalIndent(request, "", "    ")
		if err != nil {
			logger.Error("error_bot: failed to marshal request", logger.ErrAttr(err))
		} else {
			data.Request = string(req)
		}
	}

	msg := Message{
		Service: &Service{
			Id:   os.Getenv("SERVICE_ID"),
			Name: os.Getenv("SERVICE_NAME"),
		},
		Data: data,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		logger.Error("error_bot: failed to encode", logger.ErrAttr(err))
		return
	}

	webhookURL := os.Getenv("ERR_URL")
	if webhookURL == "" {
		return
	}

	resp, postErr := http.Post(webhookURL, "application/json", &buf)
	if postErr != nil {
		logger.Error("error_bot: failed to send", logger.ErrAttr(postErr))
		return
	}
	resp.Body.Close()
}
