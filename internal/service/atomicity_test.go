package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/olegsys/gophermart/internal/config"
	"github.com/olegsys/gophermart/internal/models"
)

type fakeTxManager struct {
	calls int
}

func (m *fakeTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	m.calls++
	return fn(ctx)
}

type fakeBalanceRepo struct {
	updateCalls int
	addCalls    int
	updateErr   error
}

func (r *fakeBalanceRepo) GetBalance(ctx context.Context, userID int64) (current, withdrawn float64, err error) {
	return 0, 0, nil
}

func (r *fakeBalanceRepo) UpdateBalanceWithdraw(ctx context.Context, userID int64, sum float64) error {
	r.updateCalls++
	return r.updateErr
}

func (r *fakeBalanceRepo) AddAccrual(ctx context.Context, userID int64, sum float64) error {
	r.addCalls++
	return nil
}

type fakeWithdrawalRepo struct {
	createCalls int
	createErr   error
}

func (r *fakeWithdrawalRepo) CreateWithdrawal(ctx context.Context, userID int64, order string, sum float64) error {
	r.createCalls++
	return r.createErr
}

func (r *fakeWithdrawalRepo) GetWithdrawalsByUserID(ctx context.Context, userID int64) ([]models.Withdrawal, error) {
	return nil, nil
}

func TestBalanceServiceWithdraw_DuplicateNoDoubleDebit(t *testing.T) {
	txm := &fakeTxManager{}
	balanceRepo := &fakeBalanceRepo{}
	withdrawalRepo := &fakeWithdrawalRepo{createErr: models.ErrDuplicateKey}
	svc := NewBalanceService(balanceRepo, withdrawalRepo, txm)

	err := svc.Withdraw(context.Background(), 1, "79927398713", 100)
	if err != nil {
		t.Fatalf("expected nil error for duplicate request, got %v", err)
	}
	if withdrawalRepo.createCalls != 1 {
		t.Fatalf("expected one withdrawal insert attempt, got %d", withdrawalRepo.createCalls)
	}
	if balanceRepo.updateCalls != 0 {
		t.Fatalf("expected no balance debit on duplicate, got %d", balanceRepo.updateCalls)
	}
	if txm.calls != 1 {
		t.Fatalf("expected one transaction, got %d", txm.calls)
	}
}

func TestBalanceServiceWithdraw_Success(t *testing.T) {
	txm := &fakeTxManager{}
	balanceRepo := &fakeBalanceRepo{}
	withdrawalRepo := &fakeWithdrawalRepo{}
	svc := NewBalanceService(balanceRepo, withdrawalRepo, txm)

	err := svc.Withdraw(context.Background(), 1, "79927398713", 100)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if withdrawalRepo.createCalls != 1 {
		t.Fatalf("expected one withdrawal insert, got %d", withdrawalRepo.createCalls)
	}
	if balanceRepo.updateCalls != 1 {
		t.Fatalf("expected one balance debit, got %d", balanceRepo.updateCalls)
	}
}

func TestBalanceServiceWithdraw_RejectsZeroSum(t *testing.T) {
	txm := &fakeTxManager{}
	balanceRepo := &fakeBalanceRepo{}
	withdrawalRepo := &fakeWithdrawalRepo{}
	svc := NewBalanceService(balanceRepo, withdrawalRepo, txm)

	err := svc.Withdraw(context.Background(), 1, "79927398713", 0)
	if !errors.Is(err, models.ErrInvalidWithdrawSum) {
		t.Fatalf("expected ErrInvalidWithdrawSum, got %v", err)
	}
	if withdrawalRepo.createCalls != 0 {
		t.Fatalf("expected no withdrawal insert on invalid sum, got %d", withdrawalRepo.createCalls)
	}
	if balanceRepo.updateCalls != 0 {
		t.Fatalf("expected no balance update on invalid sum, got %d", balanceRepo.updateCalls)
	}
	if txm.calls != 0 {
		t.Fatalf("expected no transaction start on invalid sum, got %d", txm.calls)
	}
}

func TestBalanceServiceWithdraw_RejectsNegativeSum(t *testing.T) {
	txm := &fakeTxManager{}
	balanceRepo := &fakeBalanceRepo{}
	withdrawalRepo := &fakeWithdrawalRepo{}
	svc := NewBalanceService(balanceRepo, withdrawalRepo, txm)

	err := svc.Withdraw(context.Background(), 1, "79927398713", -10)
	if !errors.Is(err, models.ErrInvalidWithdrawSum) {
		t.Fatalf("expected ErrInvalidWithdrawSum, got %v", err)
	}
	if withdrawalRepo.createCalls != 0 {
		t.Fatalf("expected no withdrawal insert on invalid sum, got %d", withdrawalRepo.createCalls)
	}
	if balanceRepo.updateCalls != 0 {
		t.Fatalf("expected no balance update on invalid sum, got %d", balanceRepo.updateCalls)
	}
	if txm.calls != 0 {
		t.Fatalf("expected no transaction start on invalid sum, got %d", txm.calls)
	}
}

type fakeOrderAccrualRepo struct {
	applyUserID    int64
	applyOK        bool
	applyCalls     int
	claimCalls     int
	claimOrders    []models.Order
	requeueCalls   int
	requeueCount   int64
	requeueErr     error
	requeueTimeout time.Duration
}

func (r *fakeOrderAccrualRepo) ClaimOrdersForProcessing(ctx context.Context, limit int) ([]models.Order, error) {
	r.claimCalls++
	return r.claimOrders, nil
}

func (r *fakeOrderAccrualRepo) RequeueStaleProcessing(ctx context.Context, olderThan time.Duration) (int64, error) {
	r.requeueCalls++
	r.requeueTimeout = olderThan
	return r.requeueCount, r.requeueErr
}

func (r *fakeOrderAccrualRepo) ApplyAccrualResult(ctx context.Context, number string, newStatus models.OrderStatus, accrual *float64) (int64, bool, error) {
	r.applyCalls++
	return r.applyUserID, r.applyOK, nil
}

func TestAccrualServiceCheckOrder_SkipsAlreadyAppliedCredit(t *testing.T) {
	orderRepo := &fakeOrderAccrualRepo{applyOK: false}
	balanceRepo := &fakeBalanceRepo{}
	txm := &fakeTxManager{}
	svc := NewAccrualService(orderRepo, balanceRepo, txm).(*accrualService)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"order":   "79927398713",
			"status":  "PROCESSED",
			"accrual": 150.5,
		})
	}))
	defer srv.Close()

	config.GetConfig().AccrualSystemAddress = srv.URL
	svc.checkOrder(context.Background(), "79927398713")

	if orderRepo.applyCalls != 1 {
		t.Fatalf("expected one guarded apply call, got %d", orderRepo.applyCalls)
	}
	if balanceRepo.addCalls != 0 {
		t.Fatalf("expected no accrual credit when apply is false, got %d", balanceRepo.addCalls)
	}
}

func TestAccrualServiceProcessOrders_RequeueStaleThenClaim(t *testing.T) {
	orderRepo := &fakeOrderAccrualRepo{requeueCount: 1}
	balanceRepo := &fakeBalanceRepo{}
	txm := &fakeTxManager{}
	svc := NewAccrualService(orderRepo, balanceRepo, txm).(*accrualService)

	config.GetConfig().AccrualSystemAddress = ""
	svc.processOrders(context.Background())

	if orderRepo.requeueCalls != 1 {
		t.Fatalf("expected one requeue call, got %d", orderRepo.requeueCalls)
	}
	if orderRepo.requeueTimeout != 5*time.Minute {
		t.Fatalf("expected requeue timeout 5m, got %s", orderRepo.requeueTimeout)
	}
	if orderRepo.claimCalls != 1 {
		t.Fatalf("expected one claim call, got %d", orderRepo.claimCalls)
	}
}

func TestAccrualServiceProcessOrders_RecentProcessingNotRequeued(t *testing.T) {
	orderRepo := &fakeOrderAccrualRepo{requeueCount: 0}
	balanceRepo := &fakeBalanceRepo{}
	txm := &fakeTxManager{}
	svc := NewAccrualService(orderRepo, balanceRepo, txm).(*accrualService)

	config.GetConfig().AccrualSystemAddress = ""
	svc.processOrders(context.Background())

	if orderRepo.requeueCalls != 1 {
		t.Fatalf("expected one requeue call, got %d", orderRepo.requeueCalls)
	}
	if orderRepo.claimCalls != 1 {
		t.Fatalf("expected claim to run after non-stale requeue, got %d", orderRepo.claimCalls)
	}
}
