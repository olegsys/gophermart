package service

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/olegsys/gophermart/internal/models"
)

// OrderRepo - интерфейс репозитория заказов для сервисного слоя
type OrderRepo interface {
	// CreateOrder создает новый заказ в базе данных
	CreateOrder(ctx context.Context, userID int64, number string) error
	// GetOrderByNumber получает заказ по номеру
	// Возвращает модель заказа или ошибку ErrNotFound, если заказ не найден
	GetOrderByNumber(ctx context.Context, number string) (*models.Order, error)
	// GetOrdersByUserID получает все заказы пользователя
	GetOrdersByUserID(ctx context.Context, userID int64) ([]models.Order, error)
}

// OrderService - интерфейс сервиса заказов
type OrderService interface {
	// UploadOrder загружает номер заказа для начисления баллов
	// Возвращает ошибки:
	// - ErrOrderInvalidNumberFormat - неверный формат номера (не прошел проверку Луна)
	// - ErrOrderAlreadyUploaded - заказ уже загружен этим пользователем
	// - ErrOrderConflict - заказ уже загружен другим пользователем
	UploadOrder(ctx context.Context, userID int64, number string) error
	// GetUserOrders получает список заказов пользователя
	GetUserOrders(ctx context.Context, userID int64) ([]models.Order, error)
}

type orderService struct {
	orderRepo OrderRepo
}

// NewOrderService создаёт сервис заказов
// orderRepo - репозиторий для работы с заказами
func NewOrderService(orderRepo OrderRepo) OrderService {
	return &orderService{orderRepo: orderRepo}
}

// isValidLuhn проверяет номер заказа по алгоритму Луна
// Возвращает true, если номер валиден, иначе false
func isValidLuhn(number string) bool {
	if len(number) == 0 || len(number) > 20 {
		return false
	}
	var sum int
	alt := false
	for i := len(number) - 1; i >= 0; i-- {
		digit, err := strconv.Atoi(string(number[i]))
		if err != nil {
			return false
		}
		if alt {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		alt = !alt
	}
	return sum%10 == 0
}

// UploadOrder загружает номер заказа для начисления баллов лояльности
// Проверяет валидность номера по алгоритму Луна и избегает дублирования
// Возвращает ошибки:
// - ErrOrderInvalidNumberFormat - неверный формат номера
// - ErrOrderAlreadyUploaded - заказ уже загружен этим пользователем
// - ErrOrderConflict - заказ уже загружен другим пользователем
func (s *orderService) UploadOrder(ctx context.Context, userID int64, number string) error {
	if number == "" {
		return models.ErrOrderInvalidNumberFormat
	}

	trimmedNumber := strings.TrimSpace(number)
	if !isValidLuhn(trimmedNumber) {
		return models.ErrOrderInvalidNumberFormat
	}

	existing, err := s.orderRepo.GetOrderByNumber(ctx, trimmedNumber)
	if err != nil {
		if !errors.Is(err, models.ErrOrderNotFound) {
			return err
		}
	}
	if existing != nil {
		if existing.UserID == userID {
			return models.ErrOrderAlreadyUploaded
		}
		return models.ErrOrderConflict
	}

	err = s.orderRepo.CreateOrder(ctx, userID, trimmedNumber)
	if err != nil {
		if errors.Is(err, models.ErrDuplicateKey) {
			existing, checkErr := s.orderRepo.GetOrderByNumber(ctx, trimmedNumber)
			if checkErr != nil {
				return checkErr
			}
			if existing != nil && existing.UserID != userID {
				return models.ErrOrderConflict
			}
			return models.ErrOrderAlreadyUploaded
		}
		return err
	}
	return nil
}

// GetUserOrders получает список всех заказов пользователя
func (s *orderService) GetUserOrders(ctx context.Context, userID int64) ([]models.Order, error) {
	return s.orderRepo.GetOrdersByUserID(ctx, userID)
}
