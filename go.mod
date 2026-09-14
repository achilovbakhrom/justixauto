module justixauto

go 1.27.1

// ADR-13 / T-001 approved runtime baseline. Retain these pins while owner
// scaffolds are introduced; do not add dummy imports to keep future packages.
require (
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.11.0
	github.com/labstack/echo/v4 v4.15.4
	github.com/rabbitmq/amqp091-go v1.14.0
	go.opentelemetry.io/otel v1.46.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.46.0
	go.opentelemetry.io/otel/metric v1.46.0
	go.opentelemetry.io/otel/sdk v1.46.0
	go.opentelemetry.io/otel/sdk/metric v1.46.0
	go.opentelemetry.io/otel/trace v1.46.0
	go.uber.org/zap v1.28.0
	golang.org/x/crypto v0.57.0
	gorm.io/driver/postgres v1.6.3
	gorm.io/gorm v1.31.2
)
