package domain

import "time"

// UnifiedImportReceipt describes a committed import without exposing CSV
// contents, amounts, hashes, keys or per-product quantities.
type UnifiedImportReceipt struct {
	BatchID   string    `json:"batch_id"`
	Kind      string    `json:"kind"`
	ItemCount int       `json:"item_count"`
	ActorName string    `json:"actor_name"`
	CreatedAt time.Time `json:"created_at"`
}
