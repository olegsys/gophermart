package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/olegsys/gophermart/internal/models"
)

type ctxKey string

const (
	userIDCtxKey ctxKey = "userID"
)

// AuthService - интерфейс сервиса авторизации для API слоя
type AuthService interface {
	// Register регистрирует нового пользователя и возвращает JWT токен
	Register(ctx context.Context, login, password string) (string, error)
	// Login аутентифицирует пользователя и возвращает JWT токен
	Login(ctx context.Context, login, password string) (string, error)
	// ValidateToken проверяет валидность JWT токена и возвращает ID пользователя
	ValidateToken(token string) (int64, error)
}

// OrderService - интерфейс сервиса заказов для API слоя
type OrderService interface {
	// UploadOrder загружает номер заказа для начисления баллов
	UploadOrder(ctx context.Context, userID int64, number string) error
	// GetUserOrders получает список заказов пользователя
	GetUserOrders(ctx context.Context, userID int64) ([]models.Order, error)
}

// BalanceService - интерфейс сервиса баланса для API слоя
type BalanceService interface {
	// GetBalance получает текущий баланс и сумму списаний пользователя
	GetBalance(ctx context.Context, userID int64) (*models.Balance, error)
	// Withdraw списывает средства с баланса
	Withdraw(ctx context.Context, userID int64, order string, sum float64) error
	// GetWithdrawals получает историю списаний пользователя
	GetWithdrawals(ctx context.Context, userID int64) ([]models.Withdrawal, error)
}

// Handler - интерфейс HTTP-обработчика
type Handler interface {
	// Routes возвращает маршрутизатор HTTP-хендлеров
	Routes() http.Handler
}

type handler struct {
	authService    AuthService
	orderService   OrderService
	balanceService BalanceService
}

// NewHandler создаёт HTTP-обработчик
func NewHandler(auth AuthService, order OrderService, balance BalanceService) Handler {
	return &handler{
		authService:    auth,
		orderService:   order,
		balanceService: balance,
	}
}

// Routes возвращает HTTP-маршрутизатор (mux) с зарегистрированными хендлерами API
func (h *handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/user/register", h.register)
	mux.HandleFunc("POST /api/user/login", h.login)
	mux.HandleFunc("POST /api/user/orders", h.authMiddleware(h.uploadOrder))
	mux.HandleFunc("GET /api/user/orders", h.authMiddleware(h.getOrders))
	mux.HandleFunc("GET /api/user/balance", h.authMiddleware(h.getBalance))
	mux.HandleFunc("POST /api/user/balance/withdraw", h.authMiddleware(h.withdraw))
	mux.HandleFunc("GET /api/user/withdrawals", h.authMiddleware(h.getWithdrawals))
	return mux
}

func (h *handler) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "invalid token format", http.StatusUnauthorized)
			return
		}
		userID, err := h.authService.ValidateToken(parts[1])
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userIDCtxKey, userID)
		next(w, r.WithContext(ctx))
	}
}

func (h *handler) register(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	token, err := h.authService.Register(r.Context(), creds.Login, creds.Password)
	if err != nil {
		if errors.Is(err, models.ErrLoginTaken) {
			http.Error(w, "login already taken", http.StatusConflict)
			return
		}
		slog.Error("register", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Authorization", "Bearer "+token)
	w.WriteHeader(http.StatusOK)
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	token, err := h.authService.Login(r.Context(), creds.Login, creds.Password)
	if err != nil {
		if errors.Is(err, models.ErrInvalidCredentials) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		slog.Error("login", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Authorization", "Bearer "+token)
	w.WriteHeader(http.StatusOK)
}

func (h *handler) uploadOrder(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(userIDCtxKey).(int64)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if len(body) == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	number := string(body)
	err = h.orderService.UploadOrder(r.Context(), userID, number)
	if err != nil {
		if errors.Is(err, models.ErrOrderInvalidNumberFormat) {
			http.Error(w, "invalid order number format", http.StatusUnprocessableEntity)
			return
		}
		if errors.Is(err, models.ErrOrderAlreadyUploaded) {
			w.WriteHeader(http.StatusOK)
			return
		}
		if errors.Is(err, models.ErrOrderConflict) {
			http.Error(w, "order already uploaded by another user", http.StatusConflict)
			return
		}
		slog.Error("uploadOrder", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *handler) getOrders(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(userIDCtxKey).(int64)
	orders, err := h.orderService.GetUserOrders(r.Context(), userID)
	if err != nil {
		slog.Error("getOrders", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(orders); err != nil {
		slog.Error("encode orders", "err", err)
	}
}

func (h *handler) getBalance(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(userIDCtxKey).(int64)
	balance, err := h.balanceService.GetBalance(r.Context(), userID)
	if err != nil {
		slog.Error("getBalance", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(balance); err != nil {
		slog.Error("encode balance", "err", err)
	}
}

func (h *handler) withdraw(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(userIDCtxKey).(int64)
	var req struct {
		Order string  `json:"order"`
		Sum   float64 `json:"sum"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	err := h.balanceService.Withdraw(r.Context(), userID, req.Order, req.Sum)
	if err != nil {
		if errors.Is(err, models.ErrInvalidOrderNumber) {
			http.Error(w, "invalid order number", http.StatusUnprocessableEntity)
			return
		}
		if errors.Is(err, models.ErrInvalidWithdrawSum) {
			http.Error(w, "invalid withdraw sum", http.StatusUnprocessableEntity)
			return
		}
		if errors.Is(err, models.ErrInsufficientFunds) {
			http.Error(w, "insufficient funds", http.StatusPaymentRequired)
			return
		}
		slog.Error("withdraw", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *handler) getWithdrawals(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(userIDCtxKey).(int64)
	withdrawals, err := h.balanceService.GetWithdrawals(r.Context(), userID)
	if err != nil {
		slog.Error("getWithdrawals", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if len(withdrawals) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(withdrawals); err != nil {
		slog.Error("encode withdrawals", "err", err)
	}
}
