package http

import (
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func RequestContext(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		started := time.Now()
		c.Next()
		logger.Info("http request", zap.String("request_id", id), zap.String("method", c.Request.Method), zap.String("path", c.Request.URL.Path), zap.Int("status", c.Writer.Status()), zap.Duration("duration", time.Since(started)))
	}
}

func Recovery(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if value := recover(); value != nil {
				logger.Error("panic recovered", zap.Any("panic", value), zap.ByteString("stack", debug.Stack()))
				writeError(c, &panicError{})
				c.Abort()
			}
		}()
		c.Next()
	}
}

type panicError struct{}

func (*panicError) Error() string { return "panic" }

func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "same-origin")
		c.Header("Content-Security-Policy", "default-src 'self'")
		c.Next()
	}
}

func CORS(origins []string) gin.HandlerFunc {
	allowed := map[string]bool{}
	for _, origin := range origins {
		allowed[origin] = true
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if c.Request.Method == http.MethodOptions {
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID, Idempotency-Key")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Status(http.StatusNoContent)
			c.Abort()
			return
		}
		c.Next()
	}
}

type Limiter struct {
	mu     sync.Mutex
	window time.Time
	count  int
	Limit  int
}

func (l *Limiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		l.mu.Lock()
		now := time.Now()
		if l.window.IsZero() || now.Sub(l.window) >= time.Minute {
			l.window = now
			l.count = 0
		}
		l.count++
		allowed := l.count <= l.Limit
		l.mu.Unlock()
		if !allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, ErrorResponse{Code: "RATE_LIMITED", Message: "too many requests", RequestID: requestID(c)})
			return
		}
		c.Next()
	}
}
