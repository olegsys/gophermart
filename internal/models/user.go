// Package models содержит доменные структуры
package models

import "time"

// User представляет пользователя системы лояльности
type User struct {
	ID           int64     `json:"id"`
	Login        string    `json:"login"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}
