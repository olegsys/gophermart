package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/olegsys/gophermart/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockAuthService struct{ mock.Mock }

func (m *MockAuthService) Register(ctx context.Context, login, password string) (string, error) {
	args := m.Called(ctx, login, password)
	return args.String(0), args.Error(1)
}
func (m *MockAuthService) Login(ctx context.Context, login, password string) (string, error) {
	args := m.Called(ctx, login, password)
	return args.String(0), args.Error(1)
}
func (m *MockAuthService) ValidateToken(token string) (int64, error) {
	args := m.Called(token)
	return args.Get(0).(int64), args.Error(1)
}

type MockOrderService struct{ mock.Mock }

func (m *MockOrderService) UploadOrder(ctx context.Context, userID int64, number string) error {
	return m.Called(ctx, userID, number).Error(0)
}
func (m *MockOrderService) GetUserOrders(ctx context.Context, userID int64) ([]models.Order, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]models.Order), args.Error(1)
}

type MockBalanceService struct{ mock.Mock }

func (m *MockBalanceService) GetBalance(ctx context.Context, userID int64) (*models.Balance, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Balance), args.Error(1)
}
func (m *MockBalanceService) Withdraw(ctx context.Context, userID int64, order string, sum float64) error {
	return m.Called(ctx, userID, order, sum).Error(0)
}
func (m *MockBalanceService) GetWithdrawals(ctx context.Context, userID int64) ([]models.Withdrawal, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]models.Withdrawal), args.Error(1)
}

func TestHandler_Register(t *testing.T) {
	tests := []struct {
		name           string
		body           map[string]string
		mockSetup      func(m *MockAuthService)
		expectedStatus int
	}{
		{
			name: "success", body: map[string]string{"login": "user", "password": "pass"},
			mockSetup:      func(m *MockAuthService) { m.On("Register", mock.Anything, "user", "pass").Return("token123", nil) },
			expectedStatus: http.StatusOK,
		},
		{
			name: "login taken", body: map[string]string{"login": "user", "password": "pass"},
			mockSetup: func(m *MockAuthService) {
				m.On("Register", mock.Anything, "user", "pass").Return("", models.ErrLoginTaken)
			},
			expectedStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authSvc := new(MockAuthService)
			tt.mockSetup(authSvc)
			h := NewHandler(authSvc, new(MockOrderService), new(MockBalanceService))

			b, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/user/register", bytes.NewReader(b))
			w := httptest.NewRecorder()

			h.Routes().ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, "Bearer token123", w.Header().Get("Authorization"))
			}
			authSvc.AssertExpectations(t)
		})
	}
}

func TestHandler_UploadOrder(t *testing.T) {
	tests := []struct {
		name           string
		token          string
		body           string
		mockAuthSetup  func(m *MockAuthService)
		mockOrderSetup func(m *MockOrderService)
		expectedStatus int
	}{
		{
			name: "success", token: "valid_token", body: "79927398713",
			mockAuthSetup:  func(m *MockAuthService) { m.On("ValidateToken", "valid_token").Return(int64(1), nil) },
			mockOrderSetup: func(m *MockOrderService) { m.On("UploadOrder", mock.Anything, int64(1), "79927398713").Return(nil) },
			expectedStatus: http.StatusAccepted,
		},
		{
			name: "unauthorized", token: "", body: "79927398713",
			mockAuthSetup:  func(m *MockAuthService) {},
			mockOrderSetup: func(m *MockOrderService) {},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "invalid format", token: "valid_token", body: "1234",
			mockAuthSetup: func(m *MockAuthService) { m.On("ValidateToken", "valid_token").Return(int64(1), nil) },
			mockOrderSetup: func(m *MockOrderService) {
				m.On("UploadOrder", mock.Anything, int64(1), "1234").Return(models.ErrOrderInvalidNumberFormat)
			},
			expectedStatus: http.StatusUnprocessableEntity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authSvc, orderSvc := new(MockAuthService), new(MockOrderService)
			tt.mockAuthSetup(authSvc)
			tt.mockOrderSetup(orderSvc)

			h := NewHandler(authSvc, orderSvc, new(MockBalanceService))
			req := httptest.NewRequest(http.MethodPost, "/api/user/orders", bytes.NewReader([]byte(tt.body)))
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			w := httptest.NewRecorder()

			h.Routes().ServeHTTP(w, req)
			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}
