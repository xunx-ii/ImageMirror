package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/linxunxi/image-mirror/internal/auth"
)

const (
	ContextUserID   = "userID"
	ContextUserRole = "userRole"
	ContextAPIKeyID = "apiKeyID"
)

type APIKeyLookup func(ctx context.Context, rawKey string) (userID string, keyID string, err error)

func CORS(origins []string) gin.HandlerFunc {
	config := cors.DefaultConfig()
	config.AllowOrigins = origins
	config.AllowCredentials = true
	config.AllowHeaders = []string{"Authorization", "Content-Type", "Idempotency-Key"}
	config.AllowMethods = []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"}
	return cors.New(config)
}

func JWTAuth(authService *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			Abort(c, ErrUnauthorized)
			return
		}
		claims, err := authService.ParseAccessToken(strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			Abort(c, ErrUnauthorized)
			return
		}
		if err := authService.EnsureActive(c.Request.Context(), claims.UserID); err != nil {
			Abort(c, ErrUnauthorized)
			return
		}
		c.Set(ContextUserID, claims.UserID)
		c.Set(ContextUserRole, claims.Role)
		c.Next()
	}
}

func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if role, _ := c.Get(ContextUserRole); role != "ADMIN" {
			Abort(c, ErrForbidden)
			return
		}
		c.Next()
	}
}

func FeatureEnabled(check func(context.Context) (bool, error), message string) gin.HandlerFunc {
	return func(c *gin.Context) {
		enabled, err := check(c.Request.Context())
		if err != nil {
			Abort(c, err)
			return
		}
		if !enabled {
			Abort(c, NewError(http.StatusForbidden, message, nil))
			return
		}
		c.Next()
	}
}

func APIKeyAuth(lookup APIKeyLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			Abort(c, ErrUnauthorized)
			return
		}
		raw := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		userID, keyID, err := lookup(c.Request.Context(), raw)
		if err != nil {
			Abort(c, ErrUnauthorized)
			return
		}
		c.Set(ContextUserID, userID)
		c.Set(ContextAPIKeyID, keyID)
		c.Next()
	}
}

func RateLimit(redisClient *redis.Client, limit int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limit <= 0 {
			c.Next()
			return
		}
		if shouldSkipRateLimit(c) {
			c.Next()
			return
		}
		identity := c.ClientIP()
		if userID, ok := c.Get(ContextUserID); ok {
			identity = userID.(string)
		}
		now := time.Now().Unix()
		window := now / 60
		sum := sha256.Sum256([]byte(identity))
		key := "ratelimit:" + hex.EncodeToString(sum[:8]) + ":" + strconv.FormatInt(window, 10)
		count, err := redisClient.Incr(c.Request.Context(), key).Result()
		if err == nil && count == 1 {
			_ = redisClient.Expire(c.Request.Context(), key, 2*time.Minute).Err()
		}
		if err == nil && int(count) > limit {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": gin.H{"message": "rate limit exceeded"}})
			return
		}
		c.Next()
	}
}

func shouldSkipRateLimit(c *gin.Context) bool {
	if c.Request.Method != http.MethodGet {
		return false
	}
	path := c.Request.URL.Path
	if !(strings.HasPrefix(path, "/api/images/") || strings.HasPrefix(path, "/v1/images/")) {
		return false
	}
	return strings.HasSuffix(path, "/file") || strings.HasSuffix(path, "/preview")
}

func CurrentUserID(c *gin.Context) string {
	value, _ := c.Get(ContextUserID)
	if value == nil {
		return ""
	}
	return value.(string)
}

func CurrentAPIKeyID(c *gin.Context) *string {
	value, ok := c.Get(ContextAPIKeyID)
	if !ok || value == nil {
		return nil
	}
	id := value.(string)
	return &id
}
