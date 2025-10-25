package tracing

import (
	"context"
	"net/http"

	"auth/internal/logger"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	TracerName = "go-auth-api"
)

type contextKey string

const (
	TraceIDKey      contextKey = "trace_id"
	SpanIDKey       contextKey = "span_id"
	CorrelationKey  contextKey = "correlation_id"
)

// TracingConfig holds tracing configuration
type TracingConfig struct {
	ServiceName     string
	ServiceVersion  string
	Environment     string
	JaegerEndpoint  string
	SamplingRatio   float64
	Enabled         bool
}

// Tracer wraps OpenTelemetry tracer with additional functionality
type Tracer struct {
	tracer trace.Tracer
	config *TracingConfig
	logger *logger.Logger
}

// New creates a new tracer instance
func New(config *TracingConfig, logger *logger.Logger) (*Tracer, error) {
	if !config.Enabled {
		return &Tracer{
			tracer: otel.GetTracerProvider().Tracer(TracerName),
			config: config,
			logger: logger,
		}, nil
	}

	// Create Jaeger exporter
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(config.JaegerEndpoint)))
	if err != nil {
		return nil, err
	}

	// Create tracer provider
	tp := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exp),
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
			semconv.DeploymentEnvironment(config.Environment),
		)),
		tracesdk.WithSampler(tracesdk.TraceIDRatioBased(config.SamplingRatio)),
	)

	// Register the tracer provider
	otel.SetTracerProvider(tp)

	// Set global propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer := tp.Tracer(TracerName)

	logger.Info("distributed tracing initialized",
		"service", config.ServiceName,
		"environment", config.Environment,
		"jaeger_endpoint", config.JaegerEndpoint)

	return &Tracer{
		tracer: tracer,
		config: config,
		logger: logger,
	}, nil
}

// HTTPMiddleware returns middleware for HTTP request tracing
func (t *Tracer) HTTPMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract trace context from headers
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			
			// Generate correlation ID
			correlationID := generateCorrelationID()
			ctx = context.WithValue(ctx, CorrelationKey, correlationID)
			
			// Start new span
			ctx, span := t.tracer.Start(ctx, r.Method+" "+r.URL.Path,
				trace.WithAttributes(
					semconv.HTTPMethod(r.Method),
					semconv.HTTPTarget(r.URL.Path),
					semconv.HTTPScheme(r.URL.Scheme),
					semconv.HTTPUserAgent(r.UserAgent()),
					semconv.HTTPClientIP(getRealIP(r)),
					attribute.String("correlation.id", correlationID),
				),
			)
			defer span.End()

			// Add trace info to context
			if span.SpanContext().HasTraceID() {
				ctx = context.WithValue(ctx, TraceIDKey, span.SpanContext().TraceID().String())
			}
			if span.SpanContext().HasSpanID() {
				ctx = context.WithValue(ctx, SpanIDKey, span.SpanContext().SpanID().String())
			}

			// Add tracing headers to response
			w.Header().Set("X-Trace-Id", span.SpanContext().TraceID().String())
			w.Header().Set("X-Correlation-Id", correlationID)

			// Wrap response writer to capture status code
			wrapped := &tracingResponseWriter{ResponseWriter: w, statusCode: 200}

			// Continue with request
			next.ServeHTTP(wrapped, r.WithContext(ctx))

			// Add response attributes to span
			span.SetAttributes(
				semconv.HTTPStatusCode(wrapped.statusCode),
			)

			// Set span status based on HTTP status
			if wrapped.statusCode >= 400 {
				span.RecordError(
					&HTTPError{StatusCode: wrapped.statusCode},
					trace.WithAttributes(attribute.Int("http.status_code", wrapped.statusCode)),
				)
			}
		})
	}
}

type tracingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *tracingResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// HTTPError represents an HTTP error for tracing
type HTTPError struct {
	StatusCode int
}

func (e *HTTPError) Error() string {
	return http.StatusText(e.StatusCode)
}

// StartSpan starts a new span with the given name
func (t *Tracer) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, name, opts...)
}

// TraceDatabase wraps database operations with tracing
func (t *Tracer) TraceDatabase(ctx context.Context, operation, table string) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, "db."+operation,
		trace.WithAttributes(
			semconv.DBOperation(operation),
			semconv.DBSQLTable(table),
			semconv.DBSystem("postgresql"),
		),
	)
}

// TraceAuthentication wraps authentication operations with tracing
func (t *Tracer) TraceAuthentication(ctx context.Context, authType string) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, "auth."+authType,
		trace.WithAttributes(
			attribute.String("auth.type", authType),
		),
	)
}

// TraceExternalCall wraps external service calls with tracing
func (t *Tracer) TraceExternalCall(ctx context.Context, service, operation string) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, "external."+service+"."+operation,
		trace.WithAttributes(
			attribute.String("external.service", service),
			attribute.String("external.operation", operation),
		),
	)
}

// AddSpanAttributes adds attributes to the current span
func AddSpanAttributes(ctx context.Context, attributes ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span != nil {
		span.SetAttributes(attributes...)
	}
}

// RecordError records an error in the current span
func RecordError(ctx context.Context, err error, attributes ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span != nil {
		span.RecordError(err, trace.WithAttributes(attributes...))
	}
}

// SetSpanStatus sets the status of the current span
func SetSpanStatus(ctx context.Context, code trace.StatusCode, description string) {
	span := trace.SpanFromContext(ctx)
	if span != nil {
		span.SetStatus(code, description)
	}
}

// GetTraceID extracts trace ID from context
func GetTraceID(ctx context.Context) string {
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok {
		return traceID
	}
	return ""
}

// GetCorrelationID extracts correlation ID from context
func GetCorrelationID(ctx context.Context) string {
	if correlationID, ok := ctx.Value(CorrelationKey).(string); ok {
		return correlationID
	}
	return ""
}

// generateCorrelationID generates a new correlation ID
func generateCorrelationID() string {
	return uuid.New().String()
}

// getRealIP extracts the real IP address from request
func getRealIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	
	// Fallback to RemoteAddr
	return r.RemoteAddr
}

// InjectHeaders injects tracing headers into HTTP request
func (t *Tracer) InjectHeaders(ctx context.Context, headers http.Header) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(headers))
}

// ExtractContext extracts tracing context from HTTP headers
func (t *Tracer) ExtractContext(ctx context.Context, headers http.Header) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(headers))
}