package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olegsys/gophermart/internal/database"
	"github.com/olegsys/gophermart/internal/models"
)

// OrderRepo — интерфейс репозитория заказов
type OrderRepo interface {
	// CreateOrder создает новый заказ в базе данных
	CreateOrder(ctx context.Context, userID int64, number string) error
	// GetOrderByNumber получает заказ по номеру
	// Возвращает модель заказа или ошибку ErrOrderNotFound, если заказ не найден
	GetOrderByNumber(ctx context.Context, number string) (*models.Order, error)
	// GetOrdersByUserID получает все заказы пользователя
	GetOrdersByUserID(ctx context.Context, userID int64) ([]models.Order, error)
	// UpdateOrderStatusAndAccrual обновляет статус заказа и сумму начисления
	UpdateOrderStatusAndAccrual(ctx context.Context, number string, status models.OrderStatus, accrual *float64) error
	// ClaimOrdersForProcessing выбирает заказы в работе конкурентно-безопасно
	ClaimOrdersForProcessing(ctx context.Context, limit int) ([]models.Order, error)
	// RequeueStaleProcessing возвращает зависшие PROCESSING заказы обратно в NEW
	RequeueStaleProcessing(ctx context.Context, olderThan time.Duration) (int64, error)
	// ApplyAccrualResult обновляет итоговый статус заказа, возвращает userID и признак применения
	ApplyAccrualResult(ctx context.Context, number string, newStatus models.OrderStatus, accrual *float64) (int64, bool, error)
	// GetOrdersByStatus получает заказы с указанными статусами
	GetOrdersByStatus(ctx context.Context, statuses []models.OrderStatus) ([]models.Order, error)
	// GetOrderUserID получает ID пользователя, загрузившего заказ
	GetOrderUserID(ctx context.Context, orderNumber string) (int64, error)
}

type orderRepo struct {
	pool *pgxpool.Pool
}

// NewOrderRepo создаёт репозиторий заказов
// pool - пул соединений с PostgreSQL
func NewOrderRepo(pool *pgxpool.Pool) OrderRepo {
	return &orderRepo{pool: pool}
}

func (r *orderRepo) conn(ctx context.Context) dbExecutor {
	if tx, ok := database.TxFromContext(ctx); ok {
		return tx
	}
	return r.pool
}

// CreateOrder создает новый заказ в базе данных
// Возвращает ошибку ErrDuplicateKey, если заказ с таким номером уже существует
func (r *orderRepo) CreateOrder(ctx context.Context, userID int64, number string) error {
	query := `INSERT INTO orders (number, user_id, status, uploaded_at) VALUES ($1, $2, $3, now())`
	_, err := r.conn(ctx).Exec(ctx, query, number, userID, models.OrderStatusNew)
	if err != nil {
		if pgErrCode(err) == "23505" {
			return models.ErrDuplicateKey
		}
		return fmt.Errorf("ошибка создания заказа: %w", err)
	}
	return nil
}

// GetOrderByNumber получает заказ по номеру из базы данных
// Возвращает модель заказа или ошибку ErrOrderNotFound, если заказ не найден
func (r *orderRepo) GetOrderByNumber(ctx context.Context, number string) (*models.Order, error) {
	query := `SELECT number, user_id, status, accrual, uploaded_at FROM orders WHERE number = $1`
	rows, err := r.conn(ctx).Query(ctx, query, number)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса заказа: %w", err)
	}
	defer rows.Close()

	order, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[models.Order])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrOrderNotFound
		}
		return nil, fmt.Errorf("ошибка чтения заказа: %w", err)
	}
	return &order, nil
}

// GetOrdersByUserID получает все заказы пользователя, отсортированные по дате загрузки (от новых к старым)
func (r *orderRepo) GetOrdersByUserID(ctx context.Context, userID int64) ([]models.Order, error) {
	query := `SELECT number, user_id, status, accrual, uploaded_at FROM orders WHERE user_id = $1 ORDER BY uploaded_at DESC`
	rows, err := r.conn(ctx).Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса заказов пользователя: %w", err)
	}
	defer rows.Close()

	orders, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Order])
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения заказов: %w", err)
	}
	if orders == nil {
		orders = []models.Order{}
	}
	return orders, nil
}

// UpdateOrderStatusAndAccrual обновляет статус заказа и сумму начисленных баллов
func (r *orderRepo) UpdateOrderStatusAndAccrual(ctx context.Context, number string, status models.OrderStatus, accrual *float64) error {
	query := `UPDATE orders SET status = $1, accrual = $2 WHERE number = $3`
	_, err := r.conn(ctx).Exec(ctx, query, status, accrual, number)
	if err != nil {
		return fmt.Errorf("ошибка обновления статуса заказа: %w", err)
	}
	return nil
}

// GetOrdersByStatus получает заказы с указанными статусами
func (r *orderRepo) GetOrdersByStatus(ctx context.Context, statuses []models.OrderStatus) ([]models.Order, error) {
	query := `SELECT number, user_id, status, accrual, uploaded_at FROM orders WHERE status = ANY($1)`
	rows, err := r.conn(ctx).Query(ctx, query, statuses)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса заказов по статусу: %w", err)
	}
	defer rows.Close()

	orders, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Order])
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения заказов: %w", err)
	}
	if orders == nil {
		orders = []models.Order{}
	}
	return orders, nil
}

// GetOrderUserID получает ID пользователя, загрузившего заказ по номеру
// Возвращает ошибку ErrNotFound, если заказ не найден
func (r *orderRepo) GetOrderUserID(ctx context.Context, orderNumber string) (int64, error) {
	query := `SELECT user_id FROM orders WHERE number = $1`
	var userID int64
	err := r.conn(ctx).QueryRow(ctx, query, orderNumber).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, models.ErrNotFound
		}
		return 0, fmt.Errorf("ошибка запроса user_id заказа: %w", err)
	}
	return userID, nil
}

// ClaimOrdersForProcessing atomарно выбирает и помечает заказы для обработки
func (r *orderRepo) ClaimOrdersForProcessing(ctx context.Context, limit int) ([]models.Order, error) {
	query := `
		WITH cte AS (
			SELECT number
			FROM orders
			WHERE status IN ($1, $2)
			ORDER BY uploaded_at
			FOR UPDATE SKIP LOCKED
			LIMIT $3
		)
		UPDATE orders o
		SET status = $2
		FROM cte
		WHERE o.number = cte.number
		RETURNING o.number, o.user_id, o.status, o.accrual, o.uploaded_at
	`
	rows, err := r.conn(ctx).Query(ctx, query, models.OrderStatusNew, models.OrderStatusProcessing, limit)
	if err != nil {
		return nil, fmt.Errorf("ошибка claim заказов: %w", err)
	}
	defer rows.Close()

	orders, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Order])
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения claimed заказов: %w", err)
	}
	if orders == nil {
		return []models.Order{}, nil
	}
	return orders, nil
}

// RequeueStaleProcessing возвращает в NEW заказы, которые слишком долго находятся в PROCESSING
func (r *orderRepo) RequeueStaleProcessing(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	query := `
		UPDATE orders
		SET status = $1
		WHERE status = $2 AND uploaded_at < $3
	`
	result, err := r.conn(ctx).Exec(ctx, query, models.OrderStatusNew, models.OrderStatusProcessing, cutoff)
	if err != nil {
		return 0, fmt.Errorf("ошибка возврата stale PROCESSING заказов: %w", err)
	}
	return result.RowsAffected(), nil
}

// ApplyAccrualResult применяет результат обработки заказа с защитой от повторного применения
func (r *orderRepo) ApplyAccrualResult(ctx context.Context, number string, newStatus models.OrderStatus, accrual *float64) (int64, bool, error) {
	query := `
		UPDATE orders
		SET status = $2, accrual = $3
		WHERE number = $1 AND status = $4
		RETURNING user_id
	`
	var userID int64
	err := r.conn(ctx).QueryRow(ctx, query, number, newStatus, accrual, models.OrderStatusProcessing).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("ошибка применения результата accrual: %w", err)
	}
	return userID, true, nil
}
