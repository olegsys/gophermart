package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/olegsys/gophermart/internal/api"
	"github.com/olegsys/gophermart/internal/config"
	"github.com/olegsys/gophermart/internal/database"
	"github.com/olegsys/gophermart/internal/repository"
	"github.com/olegsys/gophermart/internal/service"

	"github.com/jackc/pgx/v5/pgxpool"
)

// diContainer — контейнер зависимостей приложения
// Реализует паттерн Dependency Injection для управления зависимостями
type diContainer struct {
	pool *pgxpool.Pool

	userRepo       repository.UserRepo
	orderRepo      repository.OrderRepo
	withdrawalRepo repository.WithdrawalRepo
	balanceRepo    repository.BalanceRepo
	txManager      database.TxManager

	authService    service.AuthService
	orderService   service.OrderService
	balanceService service.BalanceService
	accrualService service.AccrualService

	handler api.Handler
}

// newDIContainer создаёт пустой DI-контейнер
func newDIContainer() *diContainer {
	return &diContainer{}
}

// Pool возвращает пул подключений к БД (создаёт при первом обращении)
func (d *diContainer) Pool() *pgxpool.Pool {
	if d.pool == nil {
		pool, err := database.NewPool(context.Background(), config.GetConfig().DatabaseURI)
		if err != nil {
			panic(fmt.Errorf("не удалось подключиться к БД: %w", err))
		}
		d.pool = pool
	}
	return d.pool
}

// UserRepo возвращает репозиторий пользователей (создаёт при первом обращении)
func (d *diContainer) UserRepo() repository.UserRepo {
	if d.userRepo == nil {
		d.userRepo = repository.NewUserRepo(d.Pool())
	}
	return d.userRepo
}

// OrderRepo возвращает репозиторий заказов (создаёт при первом обращении)
func (d *diContainer) OrderRepo() repository.OrderRepo {
	if d.orderRepo == nil {
		d.orderRepo = repository.NewOrderRepo(d.Pool())
	}
	return d.orderRepo
}

// WithdrawalRepo возвращает репозиторий списаний (создаёт при первом обращении)
func (d *diContainer) WithdrawalRepo() repository.WithdrawalRepo {
	if d.withdrawalRepo == nil {
		d.withdrawalRepo = repository.NewWithdrawalRepo(d.Pool())
	}
	return d.withdrawalRepo
}

// BalanceRepo возвращает репозиторий баланса (создаёт при первом обращении)
func (d *diContainer) BalanceRepo() repository.BalanceRepo {
	if d.balanceRepo == nil {
		d.balanceRepo = repository.NewBalanceRepo(d.Pool())
	}
	return d.balanceRepo
}

// AuthService возвращает сервис авторизации (создаёт при первом обращении)
func (d *diContainer) AuthService() service.AuthService {
	if d.authService == nil {
		d.authService = service.NewAuthService(d.UserRepo())
	}
	return d.authService
}

// OrderService возвращает сервис заказов (создаёт при первом обращении)
func (d *diContainer) OrderService() service.OrderService {
	if d.orderService == nil {
		d.orderService = service.NewOrderService(d.OrderRepo())
	}
	return d.orderService
}

// BalanceService возвращает сервис баланса (создаёт при первом обращении)
func (d *diContainer) BalanceService() service.BalanceService {
	if d.balanceService == nil {
		d.balanceService = service.NewBalanceService(d.BalanceRepo(), d.WithdrawalRepo(), d.TxManager())
	}
	return d.balanceService
}

// AccrualService возвращает сервис начисления баллов (создаёт при первом обращении)
func (d *diContainer) AccrualService() service.AccrualService {
	if d.accrualService == nil {
		d.accrualService = service.NewAccrualService(d.OrderRepo(), d.BalanceRepo(), d.TxManager())
	}
	return d.accrualService
}

// TxManager возвращает менеджер транзакций (создаёт при первом обращении)
func (d *diContainer) TxManager() database.TxManager {
	if d.txManager == nil {
		d.txManager = database.NewTxManager(d.Pool())
	}
	return d.txManager
}

// Handler возвращает HTTP-обработчик (создаёт при первом обращении)
func (d *diContainer) Handler() api.Handler {
	if d.handler == nil {
		d.handler = api.NewHandler(
			d.AuthService(),
			d.OrderService(),
			d.BalanceService(),
		)
	}
	return d.handler
}

// Close закрывает пул подключений к БД
func (d *diContainer) Close() {
	if d.pool != nil {
		slog.Info("закрытие пула подключений к БД")
		d.pool.Close()
	}
}
