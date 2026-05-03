package domain

import "errors"

var (
	ErrInvalidSaleStatus    = errors.New("invalid sale status")
	ErrSaleNotFinalized     = errors.New("sale not finalized")
	ErrSaleAlreadyCancelled = errors.New("sale already cancelled")
	ErrPaymentsMismatch     = errors.New("payments mismatch")
	ErrInvalidMoney         = errors.New("invalid money")
	ErrInvalidItem          = errors.New("invalid item")
)
