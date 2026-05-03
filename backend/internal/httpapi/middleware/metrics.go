package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	metricsOnce sync.Once

	httpInFlight *prometheus.GaugeVec
	httpTotal    *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
)

func initMetrics() {
	httpInFlight = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "sistemaemgo",
		Subsystem: "http",
		Name:      "in_flight_requests",
		Help:      "Current number of in-flight HTTP requests.",
	}, []string{"method"})

	httpTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "sistemaemgo",
		Subsystem: "http",
		Name:      "requests_total",
		Help:      "Total number of HTTP requests.",
	}, []string{"method", "route", "status"})

	httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "sistemaemgo",
		Subsystem: "http",
		Name:      "request_duration_seconds",
		Help:      "HTTP request duration in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "route", "status"})

	prometheus.MustRegister(httpInFlight, httpTotal, httpDuration)
}

func Metrics() func(http.Handler) http.Handler {
	metricsOnce.Do(initMetrics)
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			httpInFlight.WithLabelValues(r.Method).Inc()
			defer httpInFlight.WithLabelValues(r.Method).Dec()

			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)

			statusLabel := strconv.Itoa(ww.Status())
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unknown"
			}

			httpTotal.WithLabelValues(r.Method, route, statusLabel).Inc()
			httpDuration.WithLabelValues(r.Method, route, statusLabel).Observe(time.Since(start).Seconds())
		}
		return http.HandlerFunc(fn)
	}
}
