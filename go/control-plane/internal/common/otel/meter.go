package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type MeterConfig struct {
	ServiceName    string `env:"OTEL_SERVICE_NAME"`
	ServiceVersion string `env:"OTEL_SERVICE_VERSION" envDefault:"1.0.0"`
	Environment    string `env:"OTEL_ENVIRONMENT" envDefault:"development"`
	Endpoint       string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" envDefault:"localhost:4317"`
	Insecure       bool   `env:"OTEL_EXPORTER_OTLP_INSECURE" envDefault:"true"`
	Enabled        bool   `env:"OTEL_METRICS_ENABLED" envDefault:"true"`
}

type MeterProvider struct {
	provider *sdkmetric.MeterProvider
	meter    metric.Meter
	config   MeterConfig
}

func NewMeterProvider(cfg MeterConfig) (*MeterProvider, error) {
	if !cfg.Enabled {
		return &MeterProvider{
			meter:  otel.Meter(cfg.ServiceName),
			config: cfg,
		}, nil
	}

	ctx := context.Background()

	var dialOpts []grpc.DialOption
	if cfg.Insecure {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.DialContext(ctx, cfg.Endpoint, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	exporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			attribute.String("environment", cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(res),
	)

	otel.SetMeterProvider(provider)

	return &MeterProvider{
		provider: provider,
		meter:    provider.Meter(cfg.ServiceName),
		config:   cfg,
	}, nil
}

func (mp *MeterProvider) Meter() metric.Meter {
	return mp.meter
}

func (mp *MeterProvider) Shutdown(ctx context.Context) error {
	if mp.provider != nil {
		return mp.provider.Shutdown(ctx)
	}
	return nil
}

func (mp *MeterProvider) Counter(name string, opts ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return mp.meter.Int64Counter(name, opts...)
}

func (mp *MeterProvider) UpDownCounter(name string, opts ...metric.Int64UpDownCounterOption) (metric.Int64UpDownCounter, error) {
	return mp.meter.Int64UpDownCounter(name, opts...)
}

func (mp *MeterProvider) Histogram(name string, opts ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	return mp.meter.Float64Histogram(name, opts...)
}

func (mp *MeterProvider) Gauge(name string, callback metric.Int64Callback, opts ...metric.Int64ObservableGaugeOption) (metric.Int64ObservableGauge, error) {
	return mp.meter.Int64ObservableGauge(name, opts...)
}

type ServiceMetrics struct {
	RequestCounter   metric.Int64Counter
	ErrorCounter     metric.Int64Counter
	LatencyHistogram metric.Float64Histogram
	InFlightGauge    metric.Int64UpDownCounter
}

func NewServiceMetrics(meter metric.Meter, serviceName string) (*ServiceMetrics, error) {
	requestCounter, err := meter.Int64Counter(
		serviceName+"_requests_total",
		metric.WithDescription("Total number of requests"),
	)
	if err != nil {
		return nil, err
	}

	errorCounter, err := meter.Int64Counter(
		serviceName+"_errors_total",
		metric.WithDescription("Total number of errors"),
	)
	if err != nil {
		return nil, err
	}

	latencyHistogram, err := meter.Float64Histogram(
		serviceName+"_request_duration_seconds",
		metric.WithDescription("Request duration in seconds"),
	)
	if err != nil {
		return nil, err
	}

	inFlightGauge, err := meter.Int64UpDownCounter(
		serviceName+"_in_flight_requests",
		metric.WithDescription("Current number of in-flight requests"),
	)
	if err != nil {
		return nil, err
	}

	return &ServiceMetrics{
		RequestCounter:   requestCounter,
		ErrorCounter:     errorCounter,
		LatencyHistogram: latencyHistogram,
		InFlightGauge:    inFlightGauge,
	}, nil
}

func (m *ServiceMetrics) RecordRequest(ctx context.Context, method, path, status string) {
	m.RequestCounter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("method", method),
			attribute.String("path", path),
			attribute.String("status", status),
		),
	)
}

func (m *ServiceMetrics) RecordError(ctx context.Context, errorType string) {
	m.ErrorCounter.Add(ctx, 1,
		metric.WithAttributes(
			attribute.String("error_type", errorType),
		),
	)
}

func (m *ServiceMetrics) RecordLatency(ctx context.Context, seconds float64, method, path string) {
	m.LatencyHistogram.Record(ctx, seconds,
		metric.WithAttributes(
			attribute.String("method", method),
			attribute.String("path", path),
		),
	)
}

func (m *ServiceMetrics) IncInFlight(ctx context.Context) {
	m.InFlightGauge.Add(ctx, 1)
}

func (m *ServiceMetrics) DecInFlight(ctx context.Context) {
	m.InFlightGauge.Add(ctx, -1)
}
