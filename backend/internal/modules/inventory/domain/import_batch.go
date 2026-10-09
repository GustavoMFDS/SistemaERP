package domain

import "time"

// ImportBatchEntry exposes receipt metadata only. It deliberately omits the
// idempotency key, request hash, CSV rows, product prices and fiscal data.
type ImportBatchEntry struct {
	BatchID   string    `json:"batch_id"`
	ItemCount int       `json:"item_count"`
	ActorName string    `json:"actor_name"`
	CreatedAt time.Time `json:"created_at"`
}
