package models

import "errors"

var (
	ErrConflict                 = errors.New("resource already exists")
	ErrNotFound                 = errors.New("resource not found")
	ErrDuplicateKey             = errors.New("duplicate key")
	ErrInsufficientFunds        = errors.New("insufficient funds")
	ErrInvalidOrderNumber       = errors.New("invalid order number")
	ErrInvalidWithdrawSum       = errors.New("invalid withdraw sum")
	ErrLoginTaken               = errors.New("login already taken")
	ErrInvalidCredentials       = errors.New("invalid credentials")
	ErrOrderAlreadyUploaded     = errors.New("order already uploaded by this user")
	ErrOrderConflict            = errors.New("order already uploaded by another user")
	ErrOrderInvalidNumberFormat = errors.New("invalid order number format")
	ErrOrderNotFound            = errors.New("order not found")
)
