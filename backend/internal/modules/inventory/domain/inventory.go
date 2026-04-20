package domain

type MovementType string

const (
	MovementPurchase   MovementType = "purchase"
	MovementAdjustment MovementType = "adjustment"
	MovementLoss       MovementType = "loss"
	MovementDamage     MovementType = "damage"
	MovementReturn     MovementType = "return"
	MovementSale       MovementType = "sale"
)

type InventoryBalance struct {
	ProductID string  `json:"product_id"`
	QtyOnHand float64 `json:"qty_on_hand"`
}

func (b InventoryBalance) Baixar(qty float64, allowNegative bool) (InventoryBalance, error) {
	if qty <= 0 {
		return InventoryBalance{}, ErrInvalidQuantity
	}
	return b.AplicarDelta(-qty, allowNegative)
}

func (b InventoryBalance) Creditar(qty float64) (InventoryBalance, error) {
	if qty <= 0 {
		return InventoryBalance{}, ErrInvalidQuantity
	}
	return b.AplicarDelta(qty, true)
}

func (b InventoryBalance) AplicarDelta(delta float64, allowNegative bool) (InventoryBalance, error) {
	if delta == 0 {
		return InventoryBalance{}, ErrInvalidDelta
	}
	newQty := b.QtyOnHand + delta
	if !allowNegative && newQty < 0 {
		return InventoryBalance{}, ErrInsufficientStock
	}
	b.QtyOnHand = newQty
	return b, nil
}

func (t MovementType) NormalizeDelta(delta float64) (float64, error) {
	if delta == 0 {
		return 0, ErrInvalidDelta
	}
	switch t {
	case MovementLoss, MovementDamage, MovementSale:
		if delta > 0 {
			return -delta, nil
		}
		return delta, nil
	case MovementPurchase, MovementReturn:
		if delta < 0 {
			return -delta, nil
		}
		return delta, nil
	case MovementAdjustment:
		return delta, nil
	default:
		return 0, ErrInvalidMovementType
	}
}

type InventoryMovement struct {
	ID            string   `json:"id"`
	ProductID     string   `json:"product_id"`
	MovementType  string   `json:"movement_type"`
	Delta         float64  `json:"delta"`
	QtyBefore     float64  `json:"qty_before"`
	QtyAfter      float64  `json:"qty_after"`
	Reason        *string  `json:"reason"`
	ReferenceType *string  `json:"reference_type"`
	ReferenceID   *string  `json:"reference_id"`
	ActorUserID   *string  `json:"actor_user_id"`
	CreatedAt     string   `json:"created_at"`
}

func NewMovement(productID string, movementType MovementType, delta float64, before InventoryBalance, after InventoryBalance, reason *string, referenceType *string, referenceID *string, actorUserID *string, createdAt string) InventoryMovement {
	mt := string(movementType)
	return InventoryMovement{
		ProductID:     productID,
		MovementType:  mt,
		Delta:         delta,
		QtyBefore:     before.QtyOnHand,
		QtyAfter:      after.QtyOnHand,
		Reason:        reason,
		ReferenceType: referenceType,
		ReferenceID:   referenceID,
		ActorUserID:   actorUserID,
		CreatedAt:     createdAt,
	}
}
