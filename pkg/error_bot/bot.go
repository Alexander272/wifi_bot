package error_bot

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"

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
	Error   string `json:"error"`
	Request string `json:"request,omitempty"`
}

func Send(errorText string, request any) {
	webhookURL := os.Getenv("ERR_URL")
	if webhookURL == "" {
		return
	}

	var reqData []byte
	if request != nil {
		reqData, _ = json.MarshalIndent(request, "", "    ")
	}

	msg := Message{
		Service: &Service{
			Id:   os.Getenv("SERVICE_ID"),
			Name: os.Getenv("SERVICE_NAME"),
		},
		Data: &MessageData{
			Error:   errorText,
			Request: string(reqData),
		},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(msg); err != nil {
		logger.Error("error_bot: failed to encode", logger.ErrAttr(err))
		return
	}

	resp, postErr := http.Post(webhookURL, "application/json", &buf)
	if postErr != nil {
		logger.Error("error_bot: failed to send", logger.ErrAttr(postErr))
		return
	}
	resp.Body.Close()
}
