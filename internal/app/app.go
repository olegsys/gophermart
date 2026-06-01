package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/olegsys/gophermart/internal/config"
	"github.com/olegsys/gophermart/internal/database"
)

// App — структура приложения
// Содержит DI-контейнер и HTTP-сервер
type App struct {
	diContainer *diContainer
	httpServer  *http.Server
}

// New создаёт приложение и инициализирует все зависимости через DI-контейнер
func New() *App {
	a := &App{
		diContainer: newDIContainer(),
	}
	a.initDeps()
	return a
}

// initDeps выполняет инициализацию компонентов приложения
func (a *App) initDeps() {
	a.initMigrations()
	a.initHTTPServer()
}

// initMigrations применяет миграции БД перед запуском сервера
// Паникует при ошибке миграций
func (a *App) initMigrations() {
	dsn := config.GetConfig().DatabaseURI
	if dsn == "" {
		slog.Warn("DATABASE_URI не задан, миграции пропущены")
		return
	}
	if err := database.RunMigrations(dsn); err != nil {
		slog.Error("ошибка миграций", "err", err)
		panic(fmt.Errorf("ошибка миграций: %w", err))
	}
}

// initHTTPServer создаёт и настраивает HTTP-сервер
func (a *App) initHTTPServer() {
	a.httpServer = &http.Server{
		Addr:         config.GetConfig().RunAddress,
		Handler:      a.diContainer.Handler().Routes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
}

// Run запускает HTTP-сервер и обрабатывает сигналы для gracefull shutdown
// Запускает фоновый сервис начисления баллов и корректно останавливает его при завершении
func (a *App) Run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	accrualService := a.diContainer.AccrualService()
	go accrualService.Start(ctx)

	go func() {
		slog.Info("сервер запущен", "addr", config.GetConfig().RunAddress)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("ошибка сервера", "err", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	slog.Info("остановка accrual service...")
	accrualService.Stop(shutdownCtx)
	slog.Info("accrual service остановлен")

	slog.Info("остановка HTTP сервера...")
	if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("ошибка остановки HTTP сервера", "err", err)
		return err
	}
	slog.Info("HTTP сервер остановлен")

	a.diContainer.Close()
	return nil
}
