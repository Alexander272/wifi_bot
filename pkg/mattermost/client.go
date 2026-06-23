package mattermost

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(serverURL, botToken string) *Client {
	base := strings.TrimRight(serverURL, "/")
	return &Client{
		baseURL: base,
		token:   botToken,
		http:    &http.Client{},
	}
}

type Post struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	ChannelID string `json:"channel_id"`
	Message   string `json:"message"`
	RootID    string `json:"root_id"`
	CreateAt  int64  `json:"create_at"`
}

type sendPostReq struct {
	ChannelID string `json:"channel_id"`
	Message   string `json:"message"`
}

func (c *Client) SendPost(channelID, message string) error {
	body := sendPostReq{ChannelID: channelID, Message: message}
	data, _ := json.Marshal(body)

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/v4/posts", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send post: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("send post: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) GetMe() (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/v4/users/me", nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("get me: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get me: unexpected status %d", resp.StatusCode)
	}

	var u struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return "", fmt.Errorf("decode user: %w", err)
	}
	return u.ID, nil
}
