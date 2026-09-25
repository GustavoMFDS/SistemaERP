package common

import "errors"

// Shared application-level errors used across modules and HTTP handlers.
var (
	ErrInvalidCredentials     = errors.New("invalid credentials")
	ErrInactiveUser           = errors.New("user inactive")
	ErrForbidden              = errors.New("forbidden")
	ErrNotFound               = errors.New("not found")
	ErrConflict               = errors.New("conflict")
	ErrValidation             = errors.New("validation")
	ErrInsufficientStock      = errors.New("insufficient stock")
	ErrInsufficientCash       = errors.New("insufficient cash")
	ErrCashSessionClosed      = errors.New("cash session is closed")
	ErrCashSessionAlreadyOpen = errors.New("cash session already open")
	ErrPaymentsMismatch       = errors.New("payments mismatch")
	ErrPriceChanged           = errors.New("product price changed")
	ErrSaleNotFinalized       = errors.New("sale not finalized")
	ErrSaleAlreadyCancelled   = errors.New("sale already cancelled")
	ErrInvoiceAlreadyExists   = errors.New("invoice already exists")
)
