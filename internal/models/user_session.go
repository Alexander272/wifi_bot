package models

import "time"

type UserSession struct {
	ID          int64         `db:"id" json:"id"`
	UserID      string        `db:"user_id" json:"user_id"`
	Username    string        `db:"username" json:"username"`
	Code        string        `db:"code" json:"code"`
	Mac         string        `db:"mac" json:"mac"`
	IP          string        `db:"ip" json:"ip"`
	LoginAt     time.Time     `db:"login_at" json:"login_at"`
	LogoutAt    *time.Time    `db:"logout_at" json:"logout_at,omitempty"`
	IsActive    bool          `db:"is_active" json:"is_active"`
	TTLDuration time.Duration `db:"ttl_duration" json:"ttl_duration"`
}
