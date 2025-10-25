package metrics

import (
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// HTTP metrics
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)

	// Database metrics
	DatabaseQueriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "database_queries_total",
			Help: "Total number of database queries",
		},
		[]string{"operation", "table"},
	)

	DatabaseQueryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "database_query_duration_seconds",
			Help:    "Database query duration in seconds",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"operation", "table"},
	)

	// Authentication metrics
	AuthenticationAttempts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "authentication_attempts_total",
			Help: "Total number of authentication attempts",
		},
		[]string{"type", "result"},
	)

	ActiveSessions = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "active_sessions_count",
			Help: "Number of active user sessions",
		},
	)

	// System metrics
	GoRoutines = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "go_routines_count",
			Help: "Number of goroutines",
		},
	)

	MemoryUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "memory_usage_bytes",
			Help: "Memory usage in bytes",
		},
		[]string{"type"},
	)
)

func init() {
	// Register metrics with Prometheus
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDuration,
		DatabaseQueriesTotal,
		DatabaseQueryDuration,
		AuthenticationAttempts,
		ActiveSessions,
		GoRoutines,
		MemoryUsage,
	)
}

// MetricsHandler returns the Prometheus metrics handler
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

// HTTPMiddleware for collecting HTTP metrics
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Wrap the response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		
		next.ServeHTTP(wrapped, r)
		
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(wrapped.statusCode)
		
		HTTPRequestsTotal.WithLabelValues(r.Method, r.URL.Path, status).Inc()
		HTTPRequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// RecordDatabaseQuery records database query metrics
func RecordDatabaseQuery(operation, table string, duration time.Duration) {
	DatabaseQueriesTotal.WithLabelValues(operation, table).Inc()
	DatabaseQueryDuration.WithLabelValues(operation, table).Observe(duration.Seconds())
}

// RecordAuthenticationAttempt records authentication attempt metrics
func RecordAuthenticationAttempt(authType, result string) {
	AuthenticationAttempts.WithLabelValues(authType, result).Inc()
}

// UpdateActiveSession updates active sessions count
func UpdateActiveSessions(count float64) {
	ActiveSessions.Set(count)
}

// UpdateSystemMetrics updates system-level metrics
func UpdateSystemMetrics() {
	// Update goroutines count
	GoRoutines.Set(float64(runtime.NumGoroutine()))
	
	// Update memory usage
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	MemoryUsage.WithLabelValues("heap_alloc").Set(float64(m.HeapAlloc))
	MemoryUsage.WithLabelValues("heap_sys").Set(float64(m.HeapSys))
	MemoryUsage.WithLabelValues("stack_inuse").Set(float64(m.StackInuse))
}