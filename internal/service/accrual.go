package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/olegsys/gophermart/internal/config"
	"github.com/olegsys/gophermart/internal/database"
	"github.com/olegsys/gophermart/internal/models"
)

// OrderAccrualRepo - интерфейс репозитория заказов для сервиса начислений
type OrderAccrualRepo interface {
	// ClaimOrdersForProcessing атомарно выбирает и помечает заказы для обработки
	ClaimOrdersForProcessing(ctx context.Context, limit int) ([]models.Order, error)
	// RequeueStaleProcessing возвращает зависшие PROCESSING заказы обратно в NEW
	RequeueStaleProcessing(ctx context.Context, olderThan time.Duration) (int64, error)
	// ApplyAccrualResult обновляет итоговый статус заказа, возвращает userID и признак применения
	ApplyAccrualResult(ctx context.Context, number string, newStatus models.OrderStatus, accrual *float64) (int64, bool, error)
}

// BalanceAccrualRepo - интерфейс репозитория баланса для сервиса начислений
type BalanceAccrualRepo interface {
	// AddAccrual начисляет баллы на баланс пользователя
	AddAccrual(ctx context.Context, userID int64, sum float64) error
}

// AccrualService - интерфейс сервиса начисления баллов
// Обрабатывает заказы в фоновом режиме и начисляет баллы лояльности
type AccrualService interface {
	// Start запускает сервис начисления в фоновом режиме
	Start(ctx context.Context)
	// Stop останавливает сервис начисления
	Stop(ctx context.Context)
}

type accrualService struct {
	orderRepo   OrderAccrualRepo
	balanceRepo BalanceAccrualRepo
	txManager   database.TxManager
	client      *http.Client
	stopCh      chan struct{}
	stopOnce    sync.Once
}

// NewAccrualService создаёт сервис начисления баллов
// orderRepo - репозиторий для работы с заказами
// balanceRepo - репозиторий для работы с балансом
func NewAccrualService(orderRepo OrderAccrualRepo, balanceRepo BalanceAccrualRepo, txManager database.TxManager) AccrualService {
	return &accrualService{
		orderRepo:   orderRepo,
		balanceRepo: balanceRepo,
		txManager:   txManager,
		client:      &http.Client{Timeout: 10 * time.Second},
		stopCh:      make(chan struct{}),
	}
}

// Start запускает фоновый процесс обработки заказов для начисления баллов
// Запускает тикер с интервалом 5 секунд для проверки заказов в статусах NEW и PROCESSING
func (s *accrualService) Start(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				s.processOrders(ctx)
			case <-s.stopCh:
				ticker.Stop()
				return
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

// Stop останавливает фоновый процесс обработки заказов
func (s *accrualService) Stop(ctx context.Context) {
	s.stopOnce.Do(func() { close(s.stopCh) })
}

// processOrders обрабатывает заказы, находящиеся в статусах NEW и PROCESSING
func (s *accrualService) processOrders(ctx context.Context) {
	var orders []models.Order
	err := s.txManager.WithinTx(ctx, func(txCtx context.Context) error {
		requeued, requeueErr := s.orderRepo.RequeueStaleProcessing(txCtx, 5*time.Minute)
		if requeueErr != nil {
			return requeueErr
		}
		if requeued > 0 {
			slog.Info("requeued stale processing orders", "count", requeued)
		}
		var claimErr error
		orders, claimErr = s.orderRepo.ClaimOrdersForProcessing(txCtx, 50)
		return claimErr
	})
	if err != nil {
		slog.Error("не удалось получить и зарезервировать заказы для начисления", "err", err)
		return
	}
	for _, order := range orders {
		select {
		case <-ctx.Done():
			return
		default:
			s.checkOrder(ctx, order.Number)
		}
	}
}

// checkOrder проверяет статус заказа во внешней системе начисления
// Обновляет статус заказа и начисляет баллы при необходимости
func (s *accrualService) checkOrder(ctx context.Context, number string) {
	accrualAddr := config.GetConfig().AccrualSystemAddress
	if accrualAddr == "" {
		return
	}
	url := fmt.Sprintf("%s/api/orders/%s", accrualAddr, number)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Error("ошибка создания запроса", "err", err)
		return
	}
	resp, err := s.client.Do(req)
	if err != nil {
		slog.Error("ошибка запроса к accrual", "err", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := resp.Header.Get("Retry-After")
		var sleepDuration time.Duration

		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			sleepDuration = time.Duration(seconds) * time.Second
		} else {
			sleepDuration = 5 * time.Second
		}

		if sleepDuration > 0 {
			slog.Warn("получен статус 429 Too Many Requests, ждем",
				"retry_after", retryAfter,
				"sleep_duration", sleepDuration,
			)
			select {
			case <-time.After(sleepDuration):
			case <-ctx.Done():
				return
			}
		}
		return
	}

	if resp.StatusCode != http.StatusOK {
		slog.Error("accrual non-200", "status", resp.StatusCode)
		return
	}

	var result struct {
		Order   string  `json:"order"`
		Status  string  `json:"status"`
		Accrual float64 `json:"accrual"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		slog.Error("ошибка декодирования ответа accrual", "err", err)
		return
	}

	var newStatus models.OrderStatus
	switch result.Status {
	case "REGISTERED":
		newStatus = models.OrderStatusNew
	case "PROCESSING":
		newStatus = models.OrderStatusProcessing
	case "INVALID":
		newStatus = models.OrderStatusInvalid
	case "PROCESSED":
		newStatus = models.OrderStatusProcessed
	default:
		slog.Warn("неизвестный статус заказа", "status", result.Status)
		return
	}

	var accrualPtr *float64
	if newStatus == models.OrderStatusProcessed && result.Accrual > 0 {
		accrualPtr = &result.Accrual
	}

	if err := s.txManager.WithinTx(ctx, func(txCtx context.Context) error {
		userID, applied, applyErr := s.orderRepo.ApplyAccrualResult(txCtx, number, newStatus, accrualPtr)
		if applyErr != nil {
			return applyErr
		}
		if !applied {
			return nil
		}
		if newStatus == models.OrderStatusProcessed && accrualPtr != nil && *accrualPtr > 0 {
			if err := s.balanceRepo.AddAccrual(txCtx, userID, *accrualPtr); err != nil {
				return fmt.Errorf("ошибка начисления баланса: %w", err)
			}
		}
		return nil
	}); err != nil {
		slog.Error("ошибка обработки начисления", "number", number, "err", err)
	}
}
