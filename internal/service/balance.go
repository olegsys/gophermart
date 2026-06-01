package service

import (
	"context"
	"errors"

	"github.com/olegsys/gophermart/internal/database"
	"github.com/olegsys/gophermart/internal/models"
)

// BalanceRepo - интерфейс репозитория баланса для сервисного слоя
type BalanceRepo interface {
	// GetBalance получает текущий баланс и сумму списаний пользователя
	GetBalance(ctx context.Context, userID int64) (current, withdrawn float64, err error)
	// UpdateBalanceWithdraw обновляет баланс при списании средств
	UpdateBalanceWithdraw(ctx context.Context, userID int64, sum float64) error
	// AddAccrual начисляет баллы на баланс пользователя
	AddAccrual(ctx context.Context, userID int64, sum float64) error
}

// WithdrawalRepo - интерфейс репозитория списаний для сервисного слоя
type WithdrawalRepo interface {
	// CreateWithdrawal создает запись о списании средств
	CreateWithdrawal(ctx context.Context, userID int64, order string, sum float64) error
	// GetWithdrawalsByUserID получает историю списаний пользователя
	GetWithdrawalsByUserID(ctx context.Context, userID int64) ([]models.Withdrawal, error)
}

// BalanceService - интерфейс сервиса баланса
type BalanceService interface {
	// GetBalance получает текущий баланс и сумму списаний пользователя
	GetBalance(ctx context.Context, userID int64) (*models.Balance, error)
	// Withdraw списывает средства с баланса
	// Возвращает ошибки:
	// - ErrInvalidOrderNumber - неверный формат номера заказа
	// - ErrInsufficientFunds - недостаточно средств на балансе
	Withdraw(ctx context.Context, userID int64, order string, sum float64) error
	// GetWithdrawals получает историю списаний пользователя
	GetWithdrawals(ctx context.Context, userID int64) ([]models.Withdrawal, error)
}

type balanceService struct {
	balanceRepo    BalanceRepo
	withdrawalRepo WithdrawalRepo
	txManager      database.TxManager
}

// NewBalanceService создаёт сервис баланса
// balanceRepo - репозиторий для работы с балансом
// withdrawalRepo - репозиторий для работы со списаниями
func NewBalanceService(balanceRepo BalanceRepo, withdrawalRepo WithdrawalRepo, txManager database.TxManager) BalanceService {
	return &balanceService{
		balanceRepo:    balanceRepo,
		withdrawalRepo: withdrawalRepo,
		txManager:      txManager,
	}
}

// GetBalance получает текущий баланс и сумму уже списанных средств пользователя
func (s *balanceService) GetBalance(ctx context.Context, userID int64) (*models.Balance, error) {
	current, withdrawn, err := s.balanceRepo.GetBalance(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &models.Balance{Current: current, Withdrawn: withdrawn}, nil
}

// Withdraw выполняет списание средств с баланса пользователя в транзакции
func (s *balanceService) Withdraw(ctx context.Context, userID int64, order string, sum float64) error {
	if !isValidLuhn(order) {
		return models.ErrInvalidOrderNumber
	}
	if sum <= 0 {
		return models.ErrInvalidWithdrawSum
	}
	return s.txManager.WithinTx(ctx, func(txCtx context.Context) error {
		if err := s.withdrawalRepo.CreateWithdrawal(txCtx, userID, order, sum); err != nil {
			if errors.Is(err, models.ErrDuplicateKey) {
				return nil
			}
			return err
		}
		if err := s.balanceRepo.UpdateBalanceWithdraw(txCtx, userID, sum); err != nil {
			return err
		}
		return nil
	})
}

// GetWithdrawals получает историю всех списаний пользователя
func (s *balanceService) GetWithdrawals(ctx context.Context, userID int64) ([]models.Withdrawal, error) {
	return s.withdrawalRepo.GetWithdrawalsByUserID(ctx, userID)
}
