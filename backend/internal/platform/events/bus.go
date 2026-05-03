// Package events provides an in-process domain event dispatcher.
//
// Design rationale:
//   - Events are published AFTER the database transaction commits.
//     This guarantees consistency: the DB is the source of truth, and
//     handlers never see partial data from a rolled-back transaction.
//   - The dispatcher is synchronous-by-default but runs each handler in a
//     goroutine with a context deadline, so slow handlers never block the
//     HTTP response. Errors are logged but do not fail the request.
//   - For production at scale, replace the in-process bus with an outbox
//     pattern (persist events to DB inside the tx, then relay to Kafka/RabbitMQ).
//   - TenantID is carried in every event for multi-tenant routing.

package events

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/example/sistemaemgo/internal/platform"
)

// ── Event interface ───────────────────────────────────────────────────────────

// DomainEvent is the marker interface all domain events implement.
type DomainEvent interface {
	EventName() string
	OccurredAt() time.Time
	GetTenantID() string
}

// ── Concrete events ───────────────────────────────────────────────────────────

// SaleItem mirrors the minimum data handlers need from a sale item.
type SaleItemSnapshot struct {
	ProductID string
	Qty       platform.Quantity
	UnitPrice platform.Money
	CostUnit  platform.Money
}

// SaleCreatedEvent is published when a sale is successfully finalised.
type SaleCreatedEvent struct {
	SaleID    string
	TenantID  string
	SessionID string
	Total     platform.Money
	Items     []SaleItemSnapshot
	At        time.Time
}

func (e SaleCreatedEvent) EventName() string     { return "sale.created" }
func (e SaleCreatedEvent) OccurredAt() time.Time { return e.At }
func (e SaleCreatedEvent) GetTenantID() string   { return e.TenantID }

// SaleCancelledEvent is published when a sale is cancelled.
type SaleCancelledEvent struct {
	SaleID         string
	TenantID       string
	CancelledByID  string
	AmountReversed platform.Money
	At             time.Time
}

func (e SaleCancelledEvent) EventName() string     { return "sale.cancelled" }
func (e SaleCancelledEvent) OccurredAt() time.Time { return e.At }
func (e SaleCancelledEvent) GetTenantID() string   { return e.TenantID }

// InventoryDebitedEvent is published after stock is debited for a sale.
type InventoryDebitedEvent struct {
	ProductID  string
	TenantID   string
	SaleID     string
	QtyDebited platform.Quantity
	QtyAfter   platform.Quantity
	At         time.Time
}

func (e InventoryDebitedEvent) EventName() string     { return "inventory.debited" }
func (e InventoryDebitedEvent) OccurredAt() time.Time { return e.At }
func (e InventoryDebitedEvent) GetTenantID() string   { return e.TenantID }

// InventoryLowStockEvent is published when stock falls at or below minimum.
type InventoryLowStockEvent struct {
	ProductID  string
	TenantID   string
	ProductSKU string
	QtyOnHand  platform.Quantity
	MinStock   platform.Quantity
	At         time.Time
}

func (e InventoryLowStockEvent) EventName() string     { return "inventory.low_stock" }
func (e InventoryLowStockEvent) OccurredAt() time.Time { return e.At }
func (e InventoryLowStockEvent) GetTenantID() string   { return e.TenantID }

// InventoryCreditedEvent is published when stock is restored (e.g., sale cancel).
type InventoryCreditedEvent struct {
	ProductID   string
	TenantID    string
	SaleID      string
	QtyCredited platform.Quantity
	QtyAfter    platform.Quantity
	At          time.Time
}

func (e InventoryCreditedEvent) EventName() string     { return "inventory.credited" }
func (e InventoryCreditedEvent) OccurredAt() time.Time { return e.At }
func (e InventoryCreditedEvent) GetTenantID() string   { return e.TenantID }

// ── Handler ───────────────────────────────────────────────────────────────────

// Handler processes a domain event.
type Handler func(ctx context.Context, event DomainEvent) error

// ── Bus ───────────────────────────────────────────────────────────────────────

// Bus is a simple in-process publish/subscribe bus.
// It is safe for concurrent use after registration is complete.
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
	logger   *slog.Logger
	// handlerTimeout limits how long a handler may run.
	// Default: 10s. Override with WithHandlerTimeout.
	handlerTimeout time.Duration
}

// NewBus creates a new Bus. Register handlers before the server starts serving.
func NewBus(logger *slog.Logger, opts ...BusOption) *Bus {
	b := &Bus{
		handlers:       make(map[string][]Handler),
		logger:         logger,
		handlerTimeout: 10 * time.Second,
	}
	for _, o := range opts {
		o(b)
	}
	return b
}

// BusOption configures the Bus.
type BusOption func(*Bus)

// WithHandlerTimeout overrides the per-handler deadline.
func WithHandlerTimeout(d time.Duration) BusOption {
	return func(b *Bus) { b.handlerTimeout = d }
}

// Subscribe registers handler h for every event whose name equals eventName.
// Call Subscribe during application startup (not concurrently with Publish).
func (b *Bus) Subscribe(eventName string, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventName] = append(b.handlers[eventName], h)
}

// Publish dispatches the event to all registered handlers.
// Each handler runs in a separate goroutine with a deadline.
// Publish is non-blocking: it starts goroutines and returns immediately.
// Errors from handlers are logged but do NOT propagate to the caller.
//
// Invariant: always call Publish AFTER tx.Commit(), never inside a transaction.
func (b *Bus) Publish(ctx context.Context, event DomainEvent) {
	b.mu.RLock()
	hs := b.handlers[event.EventName()]
	b.mu.RUnlock()

	if len(hs) == 0 {
		return
	}

	for _, h := range hs {
		h := h // capture
		go func() {
			hCtx, cancel := context.WithTimeout(context.Background(), b.handlerTimeout)
			defer cancel()
			if err := h(hCtx, event); err != nil {
				b.logger.Error("event_handler_error",
					slog.String("event", event.EventName()),
					slog.String("tenant_id", event.GetTenantID()),
					slog.Any("err", err),
				)
			}
		}()
	}
}

// PublishAll dispatches multiple events. Convenience wrapper over Publish.
func (b *Bus) PublishAll(ctx context.Context, events []DomainEvent) {
	for _, e := range events {
		b.Publish(ctx, e)
	}
}
