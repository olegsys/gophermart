package main

import (
	"log/slog"
	"os"

	"github.com/olegsys/gophermart/internal/app"
)


func main() {
	a := app.New()

	slog.Info("приложение запущено")
	if err := a.Run(); err != nil {
		slog.Error("ошибка приложения", "err", err)
		os.Exit(1)
	}
	slog.Info("приложение остановлено")
}
