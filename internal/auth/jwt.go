// Package auth обрабатывает создание и проверку JWT
package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims - структура утверждений JWT с ID пользователя
type Claims struct {
	jwt.RegisteredClaims
	UserID int64 `json:"user_id"`
}

// TokenExp - время жизни JWT токена (24 часа)
const TokenExp = 24 * time.Hour

// GenerateToken создаёт JWT токен для указанного пользователя
// userID - ID пользователя
// secret - секретный ключ для подписи токена
// Возвращает строку токена или ошибку при подписи
func GenerateToken(userID int64, secret string) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(TokenExp)),
		},
		UserID: userID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ParseToken проверяет валидность JWT токена и возвращает утверждения
// tokenStr - строка JWT токена
// secret - секретный ключ для проверки подписи
// Возвращает Claims или ошибку при невалидном токене
func ParseToken(tokenStr, secret string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	return claims, nil
}
