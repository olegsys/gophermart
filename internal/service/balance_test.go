package service

import (
	"context"
	"testing"

	"github.com/olegsys/gophermart/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockBalanceRepo struct{ mock.Mock }

func (m *MockBalanceRepo) GetBalance(ctx context.Context, userID int64) (float64, float64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(float64), args.Get(1).(float64), args.Error(2)
}
func (m *MockBalanceRepo) UpdateBalanceWithdraw(ctx context.Context, userID int64, sum float64) error {
	return m.Called(ctx, userID, sum).Error(0)
}
func (m *MockBalanceRepo) AddAccrual(ctx context.Context, userID int64, sum float64) error {
	return m.Called(ctx, userID, sum).Error(0)
}

type MockWithdrawalRepo struct{ mock.Mock }

func (m *MockWithdrawalRepo) CreateWithdrawal(ctx context.Context, userID int64, order string, sum float64) error {
	return m.Called(ctx, userID, order, sum).Error(0)
}
func (m *MockWithdrawalRepo) GetWithdrawalsByUserID(ctx context.Context, userID int64) ([]models.Withdrawal, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]models.Withdrawal), args.Error(1)
}

type MockTxManager struct{ mock.Mock }

func (m *MockTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	m.Called(ctx, fn)
	return fn(ctx)
}

func TestBalanceService_Withdraw(t *testing.T) {
	tests := []struct {
		name          string
		userID        int64
		order         string
		sum           float64
		mockSetup     func(b *MockBalanceRepo, w *MockWithdrawalRepo, tx *MockTxManager)
		expectedError error
	}{
		{
			name: "success", userID: 1, order: "79927398713", sum: 100.0,
			mockSetup: func(b *MockBalanceRepo, w *MockWithdrawalRepo, tx *MockTxManager) {
				tx.On("WithinTx", mock.Anything, mock.Anything).Return(nil)
				w.On("CreateWithdrawal", mock.Anything, int64(1), "79927398713", 100.0).Return(nil)
				b.On("UpdateBalanceWithdraw", mock.Anything, int64(1), 100.0).Return(nil)
			},
		},
		{
			name: "invalid order number", userID: 1, order: "1234", sum: 100.0,
			mockSetup:     func(b *MockBalanceRepo, w *MockWithdrawalRepo, tx *MockTxManager) {},
			expectedError: models.ErrInvalidOrderNumber,
		},
		{
			name: "insufficient funds", userID: 1, order: "79927398713", sum: 100.0,
			mockSetup: func(b *MockBalanceRepo, w *MockWithdrawalRepo, tx *MockTxManager) {
				tx.On("WithinTx", mock.Anything, mock.Anything).Return(nil)
				w.On("CreateWithdrawal", mock.Anything, int64(1), "79927398713", 100.0).Return(nil)
				b.On("UpdateBalanceWithdraw", mock.Anything, int64(1), 100.0).Return(models.ErrInsufficientFunds)
			},
			expectedError: models.ErrInsufficientFunds,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bRepo, wRepo, txm := new(MockBalanceRepo), new(MockWithdrawalRepo), new(MockTxManager)
			tt.mockSetup(bRepo, wRepo, txm)
			svc := NewBalanceService(bRepo, wRepo, txm)

			err := svc.Withdraw(context.Background(), tt.userID, tt.order, tt.sum)
			if tt.expectedError != nil {
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
