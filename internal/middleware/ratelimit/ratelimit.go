package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"auth/internal/logger"
	"github.com/go-redis/redis/v8"
)

type RateLimiter struct {
	client *redis.Client
	logger *logger.Logger
}

type Config struct {
	Requests int           // Number of requests allowed
	Window   time.Duration // Time window
	Burst    int           // Burst capacity
}

var (
	// Different rate limits for different endpoints
	DefaultLimits = map[string]Config{
		"auth":     {Requests: 5, Window: time.Minute, Burst: 10},     // 5 req/min for auth
		"api":      {Requests: 100, Window: time.Minute, Burst: 150},  // 100 req/min for API
		"signup":   {Requests: 3, Window: time.Hour, Burst: 5},        // 3 req/hour for signup
		"login":    {Requests: 10, Window: time.Minute, Burst: 15},    // 10 req/min for login
		"profile":  {Requests: 60, Window: time.Minute, Burst: 80},    // 60 req/min for profile
	}
)

func New(redisClient *redis.Client, logger *logger.Logger) *RateLimiter {
	return &RateLimiter{
		client: redisClient,
		logger: logger,
	}
}

// Middleware returns HTTP middleware for rate limiting
func (rl *RateLimiter) Middleware(limitType string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get client identifier (IP + User-Agent hash)
			clientID := rl.getClientID(r)
			
			// Check rate limit
			allowed, remaining, resetTime, err := rl.Allow(r.Context(), clientID, limitType)
			if err != nil {
				rl.logger.Error("rate limit check failed", "error", err, "client_id", clientID)
				// Allow request on error (fail open)
				next.ServeHTTP(w, r)
				return
			}

			// Set rate limit headers
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(DefaultLimits[limitType].Requests))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

			if !allowed {
				rl.logger.Warn("rate limit exceeded", 
					"client_id", clientID, 
					"endpoint", r.URL.Path, 
					"limit_type", limitType)
				
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", strconv.Itoa(int(time.Until(resetTime).Seconds())))
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{
					"error": "rate_limit_exceeded",
					"message": "Too many requests. Please try again later.",
					"retry_after": ` + strconv.Itoa(int(time.Until(resetTime).Seconds())) + `
				}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Allow checks if a request is allowed based on sliding window algorithm
func (rl *RateLimiter) Allow(ctx context.Context, clientID, limitType string) (allowed bool, remaining int, resetTime time.Time, err error) {
	config, exists := DefaultLimits[limitType]
	if !exists {
		config = DefaultLimits["api"] // Default fallback
	}

	now := time.Now()
	window := config.Window
	maxRequests := config.Requests
	burstCapacity := config.Burst

	// Redis key for this client and limit type
	key := fmt.Sprintf("ratelimit:%s:%s", limitType, clientID)
	
	// Use Redis pipeline for atomic operations
	pipe := rl.client.Pipeline()
	
	// Remove expired entries (sliding window)
	cutoff := now.Add(-window)
	pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(cutoff.UnixNano(), 10))
	
	// Count current requests in window
	pipe.ZCard(ctx, key)
	
	// Add current request
	pipe.ZAdd(ctx, key, &redis.Z{
		Score:  float64(now.UnixNano()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})
	
	// Set expiration
	pipe.Expire(ctx, key, window+time.Minute)
	
	// Execute pipeline
	results, err := pipe.Exec(ctx)
	if err != nil {
		return false, 0, time.Time{}, err
	}

	// Get current count (before adding new request)
	currentCount := results[1].(*redis.IntCmd).Val()
	
	// Calculate reset time (start of next window)
	resetTime = now.Add(window)
	
	// Check if burst capacity allows this request
	if currentCount >= int64(burstCapacity) {
		// Remove the request we just added since it's not allowed
		rl.client.ZRem(ctx, key, fmt.Sprintf("%d", now.UnixNano()))
		return false, 0, resetTime, nil
	}
	
	// Check regular limit
	if currentCount >= int64(maxRequests) {
		// Check if we can use burst capacity
		if currentCount >= int64(burstCapacity) {
			rl.client.ZRem(ctx, key, fmt.Sprintf("%d", now.UnixNano()))
			return false, 0, resetTime, nil
		}
	}

	remaining = maxRequests - int(currentCount) - 1
	if remaining < 0 {
		remaining = 0
	}

	return true, remaining, resetTime, nil
}

// getClientID generates a unique identifier for rate limiting
func (rl *RateLimiter) getClientID(r *http.Request) string {
	// Get real IP (considering proxies)
	ip := rl.getRealIP(r)
	
	// Include user agent for better identification
	userAgent := r.Header.Get("User-Agent")
	if len(userAgent) > 100 {
		userAgent = userAgent[:100] // Truncate long user agents
	}
	
	// Create hash-based ID to avoid storing sensitive data
	return fmt.Sprintf("%s:%s", ip, hashString(userAgent))
}

// getRealIP extracts the real IP address from request
func (rl *RateLimiter) getRealIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	
	// Fallback to RemoteAddr
	parts := strings.Split(r.RemoteAddr, ":")
	if len(parts) > 0 {
		return parts[0]
	}
	
	return "unknown"
}

// hashString creates a simple hash of the string for anonymization
func hashString(s string) string {
	h := uint32(2166136261)
	for _, c := range []byte(s) {
		h ^= uint32(c)
		h *= 16777619
	}
	return fmt.Sprintf("%x", h)
}

// GetStats returns rate limiting statistics for monitoring
func (rl *RateLimiter) GetStats(ctx context.Context, limitType string) (map[string]interface{}, error) {
	pattern := fmt.Sprintf("ratelimit:%s:*", limitType)
	keys, err := rl.client.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}

	stats := map[string]interface{}{
		"active_clients": len(keys),
		"limit_type":     limitType,
		"config":         DefaultLimits[limitType],
	}

	return stats, nil
}