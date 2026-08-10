package service

import (
	"context"
	"os"
	"testing"

	"github.com/olegsys/gophermart/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type MockAuthUserRepo struct {
	mock.Mock
}

func (m *MockAuthUserRepo) CreateUser(ctx context.Context, login, passwordHash string) (int64, error) {
	args := m.Called(ctx, login, passwordHash)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockAuthUserRepo) GetUserByLogin(ctx context.Context, login string) (*models.User, error) {
	args := m.Called(ctx, login)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func TestMain(m *testing.M) {
	os.Setenv("JWT_SECRET", "test-secret-key")
	os.Exit(m.Run())
}

func TestAuthService_Register(t *testing.T) {
	tests := []struct {
		name          string
		login         string
		password      string
		mockSetup     func(m *MockAuthUserRepo)
		expectedError error
	}{
		{
			name:     "success",
			login:    "user",
			password: "pass",
			mockSetup: func(m *MockAuthUserRepo) {
				m.On("CreateUser", mock.Anything, "user", mock.AnythingOfType("string")).Return(int64(1), nil)
			},
			expectedError: nil,
		},
		{
			name:     "login taken",
			login:    "user",
			password: "pass",
			mockSetup: func(m *MockAuthUserRepo) {
				m.On("CreateUser", mock.Anything, "user", mock.AnythingOfType("string")).Return(int64(0), models.ErrDuplicateKey)
			},
			expectedError: models.ErrLoginTaken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockAuthUserRepo)
			tt.mockSetup(repo)
			svc := NewAuthService(repo)

			token, err := svc.Register(context.Background(), tt.login, tt.password)
			if tt.expectedError != nil {
				assert.ErrorIs(t, err, tt.expectedError)
				assert.Empty(t, token)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, token)
			}
			repo.AssertExpectations(t)
		})
	}
}

func TestAuthService_Login(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	require.NoError(t, err)

	tests := []struct {
		name          string
		login         string
		password      string
		mockSetup     func(m *MockAuthUserRepo)
		expectedError error
	}{
		{
			name:     "success",
			login:    "user",
			password: "password",
			mockSetup: func(m *MockAuthUserRepo) {
				m.On("GetUserByLogin", mock.Anything, "user").Return(&models.User{ID: 1, PasswordHash: string(hash)}, nil)
			},
			expectedError: nil,
		},
		{
			name:     "invalid credentials - wrong password",
			login:    "user",
			password: "wrongpass",
			mockSetup: func(m *MockAuthUserRepo) {
				m.On("GetUserByLogin", mock.Anything, "user").Return(&models.User{ID: 1, PasswordHash: string(hash)}, nil)
			},
			expectedError: models.ErrInvalidCredentials,
		},
		{
			name:     "invalid credentials - user not found",
			login:    "user",
			password: "password",
			mockSetup: func(m *MockAuthUserRepo) {
				m.On("GetUserByLogin", mock.Anything, "user").Return(nil, models.ErrNotFound)
			},
			expectedError: models.ErrInvalidCredentials,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockAuthUserRepo)
			tt.mockSetup(repo)
			svc := NewAuthService(repo)

			token, err := svc.Login(context.Background(), tt.login, tt.password)
			if tt.expectedError != nil {
				assert.ErrorIs(t, err, tt.expectedError)
				assert.Empty(t, token)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, token)
			}
			repo.AssertExpectations(t)
		})
	}
}
