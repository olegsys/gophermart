package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olegsys/gophermart/internal/database"
	"github.com/olegsys/gophermart/internal/models"
)

// WithdrawalRepo — интерфейс репозитория списаний
type WithdrawalRepo interface {
	// CreateWithdrawal создает запись о списании средств
	CreateWithdrawal(ctx context.Context, userID int64, order string, sum float64) error
	// GetWithdrawalsByUserID получает историю списаний пользователя
	GetWithdrawalsByUserID(ctx context.Context, userID int64) ([]models.Withdrawal, error)
}

type withdrawalRepo struct {
	pool *pgxpool.Pool
}

// NewWithdrawalRepo создаёт репозиторий списаний
// pool - пул соединений с PostgreSQL
func NewWithdrawalRepo(pool *pgxpool.Pool) WithdrawalRepo {
	return &withdrawalRepo{pool: pool}
}

// conn возвращает активное соединение: транзакцию, если есть, иначе пул
func (r *withdrawalRepo) conn(ctx context.Context) dbExecutor {
	if tx, ok := database.TxFromContext(ctx); ok {
		return tx
	}
	return r.pool
}

// CreateWithdrawal создает запись о списании средств в базе данных
func (r *withdrawalRepo) CreateWithdrawal(ctx context.Context, userID int64, order string, sum float64) error {
	query := `INSERT INTO withdrawals (user_id, order_number, sum, processed_at) VALUES ($1, $2, $3, now())`
	_, err := r.conn(ctx).Exec(ctx, query, userID, order, sum)
	if err != nil {
		if pgErrCode(err) == "23505" {
			return models.ErrDuplicateKey
		}
		return fmt.Errorf("ошибка создания withdrawal: %w", err)
	}
	return nil
}

// GetWithdrawalsByUserID получает историю всех списаний пользователя
func (r *withdrawalRepo) GetWithdrawalsByUserID(ctx context.Context, userID int64) ([]models.Withdrawal, error) {
	query := `SELECT order_number, sum, processed_at FROM withdrawals WHERE user_id = $1 ORDER BY processed_at DESC`
	rows, err := r.conn(ctx).Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	withdrawals, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Withdrawal])
	if err != nil {
		return nil, err
	}
	if withdrawals == nil {
		withdrawals = []models.Withdrawal{}
	}
	return withdrawals, nil
}
