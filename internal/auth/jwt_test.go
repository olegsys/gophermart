package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAndParseToken(t *testing.T) {
	secret := "test-secret"
	userID := int64(123)

	tokenStr, err := GenerateToken(userID, secret)
	require.NoError(t, err)
	assert.NotEmpty(t, tokenStr)

	claims, err := ParseToken(tokenStr, secret)
	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
}

func TestParseToken_Table(t *testing.T) {
	secret := "my-secure-secret-key"
	wrongSecret := "wrong-secret-key"
	userID := int64(100500)

	validToken, err := GenerateToken(userID, secret)
	require.NoError(t, err)

	expiredClaims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
		UserID: userID,
	}
	expiredTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredClaims)
	expiredToken, err := expiredTokenObj.SignedString([]byte(secret))
	require.NoError(t, err)

	tests := []struct {
		name           string
		tokenStr       string
		secret         string
		wantUserID     int64
		wantErrContain string
	}{
		{
			name:           "Успешный парсинг валидного токена",
			tokenStr:       validToken,
			secret:         secret,
			wantUserID:     userID,
			wantErrContain: "",
		},
		{
			name:           "Ошибка: неверный секретный ключ",
			tokenStr:       validToken,
			secret:         wrongSecret,
			wantUserID:     0,
			wantErrContain: "invalid token",
		},
		{
			name:           "Ошибка: токен просрочен",
			tokenStr:       expiredToken,
			secret:         secret,
			wantUserID:     0,
			wantErrContain: "token is expired",
		},
		{
			name:           "Ошибка: пустая строка вместо токена",
			tokenStr:       "",
			secret:         secret,
			wantUserID:     0,
			wantErrContain: "invalid token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := ParseToken(tt.tokenStr, tt.secret)

			if tt.wantErrContain != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContain)
				assert.Nil(t, claims)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, claims)
				assert.Equal(t, tt.wantUserID, claims.UserID)
			}
		})
	}
}
