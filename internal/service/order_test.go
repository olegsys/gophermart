package service

import (
	"context"
	"testing"

	"github.com/olegsys/gophermart/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockOrderRepo struct {
	mock.Mock
}

func (m *MockOrderRepo) CreateOrder(ctx context.Context, userID int64, number string) error {
	args := m.Called(ctx, userID, number)
	return args.Error(0)
}

func (m *MockOrderRepo) GetOrderByNumber(ctx context.Context, number string) (*models.Order, error) {
	args := m.Called(ctx, number)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Order), args.Error(1)
}

func (m *MockOrderRepo) GetOrdersByUserID(ctx context.Context, userID int64) ([]models.Order, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]models.Order), args.Error(1)
}

func TestIsValidLuhn(t *testing.T) {
	tests := []struct {
		name   string
		number string
		want   bool
	}{
		{"valid 1", "79927398713", true},
		{"valid 2", "12345678903", true},
		{"invalid 1", "12345678904", false},
		{"empty", "", false},
		{"too long", "123456789012345678901", false},
		{"non-numeric", "123a5678903", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidLuhn(tt.number)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOrderService_UploadOrder(t *testing.T) {
	tests := []struct {
		name          string
		userID        int64
		number        string
		mockSetup     func(m *MockOrderRepo)
		expectedError error
	}{
		{
			name:   "success",
			userID: 1,
			number: "79927398713",
			mockSetup: func(m *MockOrderRepo) {
				m.On("GetOrderByNumber", mock.Anything, "79927398713").Return(nil, models.ErrOrderNotFound)
				m.On("CreateOrder", mock.Anything, int64(1), "79927398713").Return(nil)
			},
			expectedError: nil,
		},
		{
			name:          "invalid format",
			userID:        1,
			number:        "1234",
			mockSetup:     func(m *MockOrderRepo) {},
			expectedError: models.ErrOrderInvalidNumberFormat,
		},
		{
			name:   "already uploaded by same user",
			userID: 1,
			number: "79927398713",
			mockSetup: func(m *MockOrderRepo) {
				m.On("GetOrderByNumber", mock.Anything, "79927398713").Return(&models.Order{UserID: 1}, nil)
			},
			expectedError: models.ErrOrderAlreadyUploaded,
		},
		{
			name:   "conflict - uploaded by another user",
			userID: 1,
			number: "79927398713",
			mockSetup: func(m *MockOrderRepo) {
				m.On("GetOrderByNumber", mock.Anything, "79927398713").Return(&models.Order{UserID: 2}, nil)
			},
			expectedError: models.ErrOrderConflict,
		},
		{
			name:   "race condition - duplicate key on create",
			userID: 1,
			number: "79927398713",
			mockSetup: func(m *MockOrderRepo) {
				m.On("GetOrderByNumber", mock.Anything, "79927398713").Return(nil, models.ErrOrderNotFound).Once()
				m.On("CreateOrder", mock.Anything, int64(1), "79927398713").Return(models.ErrDuplicateKey)
				m.On("GetOrderByNumber", mock.Anything, "79927398713").Return(&models.Order{UserID: 2}, nil).Once()
			},
			expectedError: models.ErrOrderConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockOrderRepo)
			tt.mockSetup(repo)
			svc := NewOrderService(repo)

			err := svc.UploadOrder(context.Background(), tt.userID, tt.number)
			if tt.expectedError != nil {
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
			repo.AssertExpectations(t)
		})
	}
}
