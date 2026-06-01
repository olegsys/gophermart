package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olegsys/gophermart/internal/database"
	"github.com/olegsys/gophermart/internal/models"
)

// BalanceRepo — интерфейс репозитория баланса
type BalanceRepo interface {
	// GetBalance получает текущий баланс и сумму списаний пользователя
	GetBalance(ctx context.Context, userID int64) (current, withdrawn float64, err error)
	// UpdateBalanceWithdraw списывает сумму со счёта атомарно
	// Возвращает ошибку ErrInsufficientFunds при недостаточном балансе
	UpdateBalanceWithdraw(ctx context.Context, userID int64, sum float64) error
	// AddAccrual начисляет баллы на счёт пользователя
	AddAccrual(ctx context.Context, userID int64, sum float64) error
}

type balanceRepo struct {
	pool *pgxpool.Pool
}

// NewBalanceRepo создаёт репозиторий баланса
func NewBalanceRepo(pool *pgxpool.Pool) BalanceRepo {
	return &balanceRepo{pool: pool}
}

// dbExecutor — общий интерфейс для pool и tx
type dbExecutor interface {
	Query(ctx context.Context, query string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, query string, args ...any) pgx.Row
	Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error)
}

// conn возвращает активное соединение: транзакцию, если есть, иначе пул
func (r *balanceRepo) conn(ctx context.Context) dbExecutor {
	if tx, ok := database.TxFromContext(ctx); ok {
		return tx
	}
	return r.pool
}

// GetBalance получает текущий баланс и сумму списаний пользователя
// При отсутствии записи возвращает нулевые значения без ошибки
func (r *balanceRepo) GetBalance(ctx context.Context, userID int64) (current, withdrawn float64, err error) {
	query := `SELECT current, withdrawn FROM user_balance WHERE user_id = $1`
	err = r.conn(ctx).QueryRow(ctx, query, userID).Scan(&current, &withdrawn)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	return current, withdrawn, nil
}

// UpdateBalanceWithdraw списывает сумму со счёта атомарно
// Проверяет наличие достаточных средств перед списанием
// Возвращает ошибку ErrInsufficientFunds при недостаточном балансе
func (r *balanceRepo) UpdateBalanceWithdraw(ctx context.Context, userID int64, sum float64) error {
	query := `
		UPDATE user_balance
		SET current = current - $1, withdrawn = withdrawn + $1
		WHERE user_id = $2 AND current >= $1
	`
	result, err := r.conn(ctx).Exec(ctx, query, sum, userID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return models.ErrInsufficientFunds
	}
	return nil
}

// AddAccrual начисляет баллы на счёт пользователя
// Если запись для пользователя не существует, создаёт новую
func (r *balanceRepo) AddAccrual(ctx context.Context, userID int64, sum float64) error {
	query := `
		INSERT INTO user_balance (user_id, current, withdrawn)
		VALUES ($1, $2, 0)
		ON CONFLICT (user_id) DO UPDATE SET current = user_balance.current + $2
	`
	_, err := r.conn(ctx).Exec(ctx, query, userID, sum)
	return err
}
