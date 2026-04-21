package observability

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"

	"github.com/example/sistemaemgo/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func Setup(ctx context.Context, cfg config.Config, logger *slog.Logger) (func(context.Context) error, error) {
	_ = logger
	if !cfg.OTelEnabled {
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.DeploymentEnvironment(cfg.Env),
		),
	)
	if err != nil {
		return nil, err
	}

	var exp sdktrace.SpanExporter
	switch strings.ToLower(strings.TrimSpace(cfg.OTelExporter)) {
	case "stdout":
		exp, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	case "otlp":
		exp, err = newOTLPHTTPExporter(ctx, cfg.OTelOTLPEndpoint)
	default:
		err = errors.New("unsupported OTEL_EXPORTER")
	}
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

func newOTLPHTTPExporter(ctx context.Context, endpoint string) (sdktrace.SpanExporter, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil, errors.New("OTEL_EXPORTER_OTLP_ENDPOINT is empty")
	}
	// Accept either host:port or a full URL (http(s)://host:port[/path]).
	if strings.Contains(endpoint, "://") {
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		opts := []otlptracehttp.Option{}
		if host := strings.TrimSpace(u.Host); host != "" {
			opts = append(opts, otlptracehttp.WithEndpoint(host))
		}
		if p := strings.TrimSpace(u.EscapedPath()); p != "" && p != "/" {
			opts = append(opts, otlptracehttp.WithURLPath(p))
		}
		if strings.EqualFold(u.Scheme, "http") {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		return otlptracehttp.New(ctx, opts...)
	}
	return otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
}
