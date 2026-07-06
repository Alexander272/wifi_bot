package models

import "time"

type WifiSession struct {
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Code      string    `json:"code"`
	Mac       string    `json:"mac"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"created_at"`
}

type MattermostRequest struct {
	Token    string `json:"token"`
	UserID   string `json:"user_id"`
	UserName string `json:"user_name"`
	Command  string `json:"command"`
	Text     string `json:"text"`
}

type MattermostResponse struct {
	ResponseType string `json:"response_type"`
	Text         string `json:"text"`
}
