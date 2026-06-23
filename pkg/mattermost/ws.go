package mattermost

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"wifi_bot/pkg/logger"
)

type EventData struct {
	ChannelType string `json:"channel_type"`
	Post        string `json:"post"`
	SenderName  string `json:"sender_name"`
}

type wsEvent struct {
	Event string    `json:"event"`
	Data  EventData `json:"data"`
}

type Event struct {
	ChannelType string
	Post        Post
}

type EventHandler func(Event)

type WSClient struct {
	conn     *websocket.Conn
	baseURL  string
	token    string
	handler  EventHandler
	userID   string
	dialer   *websocket.Dialer
}

func NewWSClient(serverURL, token string) *WSClient {
	base := strings.TrimRight(serverURL, "/")
	return &WSClient{
		baseURL: base,
		token:   token,
		dialer:  &websocket.Dialer{HandshakeTimeout: 10 * time.Second},
	}
}

func (w *WSClient) SetHandler(handler EventHandler) {
	w.handler = handler
}

func (w *WSClient) SetUserID(uid string) {
	w.userID = uid
}

func (w *WSClient) Connect(ctx context.Context) error {
	scheme := "wss"
	if strings.HasPrefix(w.baseURL, "http://") {
		scheme = "ws"
	}
	u := url.URL{Scheme: scheme, Host: strings.TrimPrefix(strings.TrimPrefix(w.baseURL, "https://"), "http://"), Path: "/api/v4/websocket"}

	conn, _, err := w.dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	w.conn = conn

	if err := w.authenticate(); err != nil {
		conn.Close()
		return fmt.Errorf("auth: %w", err)
	}

	return nil
}

func (w *WSClient) authenticate() error {
	auth := map[string]any{
		"seq":     1,
		"action":  "authentication_challenge",
		"data":    map[string]string{"token": w.token},
	}
	return w.conn.WriteJSON(auth)
}

func (w *WSClient) Listen(ctx context.Context) error {
	for {
		_, raw, err := w.conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var msg struct {
			Event string          `json:"event"`
			Data  json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		if msg.Event != "posted" || len(msg.Data) == 0 {
			continue
		}

		var data EventData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			continue
		}

		if data.ChannelType != "D" || data.Post == "" {
			continue
		}

		var post Post
		if err := json.Unmarshal([]byte(data.Post), &post); err != nil {
			continue
		}

		if post.UserID == w.userID {
			continue
		}

		w.handler(Event{
			ChannelType: data.ChannelType,
			Post:        post,
		})
	}
}

func (w *WSClient) Run(ctx context.Context) {
	backoff := 1 * time.Second
	maxBackoff := 60 * time.Second

	for {
		if err := w.Connect(ctx); err != nil {
			logger.Warn("mattermost ws: connect error, retry in %v", logger.ErrAttr(err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			continue
		}
		backoff = 1 * time.Second

		logger.Info("mattermost ws: connected")

		done := make(chan error, 1)
		go func() {
			done <- w.Listen(ctx)
		}()

		select {
		case err := <-done:
			if err != nil {
				logger.Warn("mattermost ws: listen error, reconnecting...", logger.ErrAttr(err))
			}
		case <-ctx.Done():
			if w.conn != nil {
				w.conn.Close()
			}
			<-done
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}


