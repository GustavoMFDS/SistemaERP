package domain

import "errors"

var (
	ErrProductInactive  = errors.New("product inactive")
	ErrInvalidQuantity  = errors.New("invalid quantity")
	ErrInvalidPrice     = errors.New("invalid price")
	ErrInvalidDelta     = errors.New("invalid delta")
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrInvalidMovementType = errors.New("invalid movement type")
)
