// Package config предоставляет инструменты для инициализации и управления
// конфигурационными параметрами приложения. Пакет поддерживает многоуровневую
// настройку с использованием флагов командной строки и переменных окружения,
// автоматически обеспечивая приоритет переменных окружения над флагами
package config

import (
	"flag"
	"os"
	"sync"
)

// Config содержит параметры конфигурации приложения
// Все поля могут быть настроены через флаги командной строки
// или переопределены соответствующими переменными окружения
type Config struct {
	// RunAddress — сетевой адрес и порт для запуска HTTP-сервера (например, "localhost:8080")
	RunAddress string

	// DatabaseURI — строка подключения (строка соединения DSN) к базе данных
	DatabaseURI string

	// AccrualSystemAddress — адрес внешней системы расчета баллов лояльности
	AccrualSystemAddress string

	// JWTSecret — секретный ключ для подписи и валидации JWT-токенов авторизации
	JWTSecret string
}

var (
	instance *Config
	once     sync.Once
)

// GetConfig инициализирует и возвращает конфигурацию приложения
// Функция сначала парсит флаги командной строки, а затем переопределяет
// значения из переменных окружения, если они установлены
// Порядок приоритета: Переменные окружения > Флаги командной строки > Дефолтные значения
func GetConfig() *Config {
	once.Do(func() {
		cfg := &Config{}
		flag.StringVar(&cfg.RunAddress, "a", "localhost:8080", "server address")
		flag.StringVar(&cfg.DatabaseURI, "d", "", "database URI")
		flag.StringVar(&cfg.AccrualSystemAddress, "r", "", "accrual system address")
		flag.StringVar(&cfg.JWTSecret, "s", "secretKey", "jwt token secret key")
		flag.Parse()
		if val := os.Getenv("RUN_ADDRESS"); val != "" {
			cfg.RunAddress = val
		}
		if val := os.Getenv("DATABASE_URI"); val != "" {
			cfg.DatabaseURI = val
		}
		if val := os.Getenv("ACCRUAL_SYSTEM_ADDRESS"); val != "" {
			cfg.AccrualSystemAddress = val
		}
		if val := os.Getenv("JWT_SECRET"); val != "" {
			cfg.JWTSecret = val
		}
		instance = cfg
	})

	return instance
}
