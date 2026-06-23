package models

import "errors"

var (
	ErrSessionNotFound  = errors.New("session not found")
	ErrCodeExpired      = errors.New("code expired")
	ErrCodeInvalid      = errors.New("invalid code")
	ErrCodeAlreadyUsed  = errors.New("code already used on another device")
	ErrMikrotikAuth     = errors.New("mikrotik auth failed")
)
