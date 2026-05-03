// Package tracing provides OpenTelemetry span helpers for the application.
//
// Usage pattern — in a service method:
//
//	ctx, span := tracing.StartSpan(ctx, "sales.CreateAndFinalize")
//	defer span.End()
//	tracing.SetAttr(span, "tenant_id", tenantID)
//	if err != nil { tracing.RecordError(span, err); return err }
//
// Usage pattern — in a repository method:
//
//	ctx, span := tracing.StartDBSpan(ctx, "sales_repo.InsertSale")
//	defer span.End()
//
// The package is a thin wrapper; direct use of go.opentelemetry.io/otel is
// also fine. This wrapper reduces boilerplate and ensures consistent attribute names.

package tracing

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	tracerName = "sistemaemgo"
)

// StartSpan starts a new span in the application/service layer.
func StartSpan(ctx context.Context, operationName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return otel.GetTracerProvider().
		Tracer(tracerName).
		Start(ctx, operationName, opts...)
}

// StartDBSpan starts a span for a database operation with db.system attribute.
func StartDBSpan(ctx context.Context, operationName string) (context.Context, trace.Span) {
	return otel.GetTracerProvider().
		Tracer(tracerName).
		Start(ctx, operationName,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attribute.String("db.system", "postgresql")),
		)
}

// StartCacheSpan starts a span for a Redis cache operation.
func StartCacheSpan(ctx context.Context, operationName string) (context.Context, trace.Span) {
	return otel.GetTracerProvider().
		Tracer(tracerName).
		Start(ctx, operationName,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attribute.String("db.system", "redis")),
		)
}

// SetAttr sets a string attribute on the span. Variadic for convenience.
func SetAttr(span trace.Span, key, value string) {
	span.SetAttributes(attribute.String(key, value))
}

// SetAttrs sets multiple string attributes.
func SetAttrs(span trace.Span, attrs map[string]string) {
	for k, v := range attrs {
		span.SetAttributes(attribute.String(k, v))
	}
}

// SetIntAttr sets an integer attribute.
func SetIntAttr(span trace.Span, key string, value int) {
	span.SetAttributes(attribute.Int(key, value))
}

// SetFloatAttr sets a float64 attribute.
func SetFloatAttr(span trace.Span, key string, value float64) {
	span.SetAttributes(attribute.Float64(key, value))
}

// RecordError marks the span as errored and records the error.
// It does NOT call span.End() — the caller must defer that.
func RecordError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// SetTenantID is a convenience wrapper for the tenant_id attribute.
// Consistent naming across all spans is critical for filtering in Jaeger/Tempo.
func SetTenantID(span trace.Span, tenantID string) {
	span.SetAttributes(attribute.String("tenant.id", tenantID))
}

// SetUserID is a convenience wrapper for the user_id attribute.
func SetUserID(span trace.Span, userID string) {
	span.SetAttributes(attribute.String("user.id", userID))
}

// ── Metrics (Prometheus) ──────────────────────────────────────────────────────
// Keep Prometheus metrics here to co-locate observability concerns.

var (
	// SalesTotal counts completed sales.
	SalesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "sistemaemgo",
		Subsystem: "sales",
		Name:      "total",
		Help:      "Total number of completed sales.",
	}, []string{"tenant_id", "payment_method"})

	// SalesRevenueTotal tracks gross revenue.
	SalesRevenueTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "sistemaemgo",
		Subsystem: "sales",
		Name:      "revenue_total_brl",
		Help:      "Total gross revenue in BRL.",
	}, []string{"tenant_id"})

	// SalesCancellationsTotal counts cancellations.
	SalesCancellationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "sistemaemgo",
		Subsystem: "sales",
		Name:      "cancellations_total",
		Help:      "Total number of cancelled sales.",
	}, []string{"tenant_id"})

	// InventoryDebits counts stock debits.
	InventoryDebits = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "sistemaemgo",
		Subsystem: "inventory",
		Name:      "debits_total",
		Help:      "Total number of stock debit operations.",
	}, []string{"tenant_id"})

	// LowStockAlerts counts low-stock events.
	LowStockAlerts = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "sistemaemgo",
		Subsystem: "inventory",
		Name:      "low_stock_alerts_total",
		Help:      "Total number of low-stock threshold breaches.",
	}, []string{"tenant_id"})

	// CacheHitRate tracks cache hit/miss ratio.
	CacheHitsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "sistemaemgo",
		Subsystem: "cache",
		Name:      "hits_total",
		Help:      "Total number of cache hits.",
	}, []string{"entity"})

	CacheMissesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "sistemaemgo",
		Subsystem: "cache",
		Name:      "misses_total",
		Help:      "Total number of cache misses.",
	}, []string{"entity"})

	// SaleDuration tracks end-to-end sale processing time.
	SaleDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "sistemaemgo",
		Subsystem: "sales",
		Name:      "duration_seconds",
		Help:      "End-to-end sale processing duration.",
		Buckets:   []float64{.01, .025, .05, .1, .25, .5, 1, 2.5},
	}, []string{"tenant_id", "status"})

	// DBQueryDuration tracks repository query times.
	DBQueryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "sistemaemgo",
		Subsystem: "db",
		Name:      "query_duration_seconds",
		Help:      "Database query duration.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"operation"})
)
