package service

import (
	"context"
	"errors"

	"github.com/olegsys/gophermart/internal/auth"
	"github.com/olegsys/gophermart/internal/config"
	"github.com/olegsys/gophermart/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// AuthUserRepo - интерфейс репозитория пользователей для сервиса авторизации
type AuthUserRepo interface {
	// CreateUser создаёт нового пользователя в базе данных
	// Возвращает ID созданного пользователя
	CreateUser(ctx context.Context, login, passwordHash string) (int64, error)
	// GetUserByLogin получает пользователя по логину
	// Возвращает модель пользователя или ошибку ErrNotFound, если пользователь не найден
	GetUserByLogin(ctx context.Context, login string) (*models.User, error)
}

// AuthService - интерфейс сервиса авторизации
type AuthService interface {
	// Register регистрирует нового пользователя и возвращает JWT токен
	Register(ctx context.Context, login, password string) (string, error)
	// Login аутентифицирует пользователя и возвращает JWT токен
	// Возвращает ошибку ErrInvalidCredentials при неверных учетных данных
	Login(ctx context.Context, login, password string) (string, error)
	// ValidateToken проверяет валидность JWT токена и возвращает ID пользователя
	ValidateToken(token string) (int64, error)
}

type authService struct {
	userRepo AuthUserRepo
}

// NewAuthService создаёт сервис авторизации
// userRepo - репозиторий для работы с пользователями
func NewAuthService(userRepo AuthUserRepo) AuthService {
	return &authService{userRepo: userRepo}
}

// Register регистрирует нового пользователя в системе
// Возвращает JWT токен для аутентификации или ошибку при:
// - ErrDuplicateKey - логин уже занят
// - bcrypt error - ошибка генерации хэша пароля
func (s *authService) Register(ctx context.Context, login, password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	userID, err := s.userRepo.CreateUser(ctx, login, string(hash))
	if err != nil {
		if errors.Is(err, models.ErrDuplicateKey) {
			return "", models.ErrLoginTaken
		}
		return "", err
	}
	token, err := auth.GenerateToken(userID, config.GetConfig().JWTSecret)
	if err != nil {
		return "", err
	}
	return token, nil
}

// Login аутентифицирует пользователя по логину и паролю
// Возвращает JWT токен или ошибку ErrInvalidCredentials при неверных учетных данных
func (s *authService) Login(ctx context.Context, login, password string) (string, error) {
	user, err := s.userRepo.GetUserByLogin(ctx, login)
	if err != nil {
		return "", models.ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", models.ErrInvalidCredentials
	}
	token, err := auth.GenerateToken(user.ID, config.GetConfig().JWTSecret)
	if err != nil {
		return "", err
	}
	return token, nil
}

// ValidateToken проверяет валидность JWT токена
// Возвращает ID пользователя из токена или ошибку при невалидном токене
func (s *authService) ValidateToken(tokenStr string) (int64, error) {
	claims, err := auth.ParseToken(tokenStr, config.GetConfig().JWTSecret)
	if err != nil {
		return 0, err
	}
	return claims.UserID, nil
}
