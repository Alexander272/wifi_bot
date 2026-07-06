package models

import "time"

type AuthLog struct {
	ID        string    `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`
	Username  string    `json:"username" db:"username"`
	Action    string    `json:"action" db:"action"`
	Code      *string   `json:"code,omitempty" db:"code"`
	Mac       *string   `json:"mac,omitempty" db:"mac"`
	IP        *string   `json:"ip,omitempty" db:"ip"`
	Metadata  *string   `json:"metadata,omitempty" db:"metadata"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

func StrPtr(s string) *string { return &s }
