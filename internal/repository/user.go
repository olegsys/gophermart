package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olegsys/gophermart/internal/models"
)

// UserRepo — интерфейс репозитория пользователей
type UserRepo interface {
	// CreateUser создаёт нового пользователя в базе данных
	// Возвращает ID созданного пользователя
	CreateUser(ctx context.Context, login, passwordHash string) (int64, error)
	// GetUserByLogin получает пользователя по логину
	// Возвращает модель пользователя или ошибку, если пользователь не найден
	GetUserByLogin(ctx context.Context, login string) (*models.User, error)
	// GetUserByID получает пользователя по ID
	// Возвращает модель пользователя или ошибку, если пользователь не найден
	GetUserByID(ctx context.Context, id int64) (*models.User, error)
}

type userRepo struct {
	pool *pgxpool.Pool
}

// NewUserRepo создаёт репозиторий пользователей
// pool - пул соединений с PostgreSQL
func NewUserRepo(pool *pgxpool.Pool) UserRepo {
	return &userRepo{pool: pool}
}

// pgErrCode возвращает код ошибки PostgreSQL, если это *pgconn.PgError
func pgErrCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// CreateUser создаёт нового пользователя в базе данных
// Возвращает ID созданного пользователя или ошибку ErrDuplicateKey, если логин уже занят
func (r *userRepo) CreateUser(ctx context.Context, login, passwordHash string) (int64, error) {
	query := `INSERT INTO users (login, password_hash) VALUES ($1, $2) RETURNING id`
	var id int64
	err := r.pool.QueryRow(ctx, query, login, passwordHash).Scan(&id)
	if err != nil {
		if pgErrCode(err) == "23505" {
			return 0, models.ErrDuplicateKey
		}
		return 0, fmt.Errorf("ошибка создания пользователя: %w", err)
	}
	return id, nil
}

// GetUserByLogin получает пользователя по логину из базы данных
// Возвращает модель пользователя или ошибку, если пользователь не найден
func (r *userRepo) GetUserByLogin(ctx context.Context, login string) (*models.User, error) {
	query := `SELECT id, login, password_hash, created_at FROM users WHERE login = $1`
	rows, err := r.pool.Query(ctx, query, login)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса пользователя: %w", err)
	}
	defer rows.Close()

	user, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[models.User])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("пользователь не найден")
		}
		return nil, fmt.Errorf("ошибка чтения пользователя: %w", err)
	}
	return &user, nil
}

// GetUserByID получает пользователя по ID из базы данных
// Возвращает модель пользователя или ошибку, если пользователь не найден
func (r *userRepo) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	query := `SELECT id, login, password_hash, created_at FROM users WHERE id = $1`
	rows, err := r.pool.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса пользователя по id: %w", err)
	}
	defer rows.Close()

	user, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[models.User])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("пользователь не найден")
		}
		return nil, fmt.Errorf("ошибка чтения пользователя: %w", err)
	}
	return &user, nil
}
