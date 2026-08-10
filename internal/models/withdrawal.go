package models

import "time"

// Withdrawal представляет запись о списании баллов
type Withdrawal struct {
	// Order - номер заказа, за который были списаны баллы
	Order string `json:"order" db:"order_number"`
	// Sum - сумма списания
	Sum float64 `json:"sum"`
	// ProcessedAt - дата и время списания
	ProcessedAt time.Time `json:"processed_at"`
}
