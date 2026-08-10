package models

// Balance представляет текущий баланс пользователя
type Balance struct {
	// Current - текущий баланс (доступные баллы для списания)
	Current float64 `json:"current"`
	// Withdrawn - сумма уже списанных баллов
	Withdrawn float64 `json:"withdrawn"`
}
