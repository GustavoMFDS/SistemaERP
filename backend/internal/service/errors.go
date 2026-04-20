package service

import "errors"

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrInactiveUser = errors.New("user inactive")
var ErrForbidden = errors.New("forbidden")
var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrValidation = errors.New("validation")
var ErrInsufficientStock = errors.New("insufficient stock")
var ErrCashSessionClosed = errors.New("cash session is closed")
var ErrPaymentsMismatch = errors.New("payments mismatch")
var ErrSaleNotFinalized = errors.New("sale not finalized")
var ErrSaleAlreadyCancelled = errors.New("sale already cancelled")
var ErrInvoiceAlreadyExists = errors.New("invoice already exists")
