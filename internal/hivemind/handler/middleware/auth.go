package middleware

import (
	"crypto/subtle"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthConfig holds configuration for Bearer token authentication.
// Modeled after OpenClaw's gateway/auth.ts — supports token + local bypass.
type AuthConfig struct {
	// Enabled controls whether authentication is enforced.
	Enabled bool `json:"enabled"`

	// Token is the expected Bearer token value.
	// Can also be set via EIDOLON_GATEWAY_TOKEN environment variable.
	Token string `json:"token"`
}

// ResolveToken returns the effective token, checking env vars as fallback.
func (c *AuthConfig) ResolveToken() string {
	if c.Token != "" {
		return c.Token
	}
	return os.Getenv("EIDOLON_GATEWAY_TOKEN")
}

// BearerAuth returns a Gin middleware that enforces Bearer token authentication.
//
// Security features (aligned with OpenClaw auth.ts):
//   - Uses crypto/subtle.ConstantTimeCompare to prevent timing attacks
//   - Skips auth for local loopback requests when allowLocal is true
//   - Whitelists /healthz and /version paths
func BearerAuth(cfg *AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.Enabled {
			c.Next()
			return
		}

		token := cfg.ResolveToken()
		if token == "" {
			c.Next()
			return
		}

		// Whitelist paths that don't require auth.
		path := c.Request.URL.Path
		if path == "/healthz" || path == "/version" {
			c.Next()
			return
		}

		// Allow local loopback requests (OpenClaw: isLocalDirectRequest).
		if isLocalRequest(c.Request) {
			c.Next()
			return
		}

		// Extract Bearer token from Authorization header.
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "missing Authorization header",
					"type":    "authentication_error",
				},
			})
			return
		}

		const prefix = "Bearer "
		if !strings.HasPrefix(authHeader, prefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "invalid Authorization header format, expected 'Bearer <token>'",
					"type":    "authentication_error",
				},
			})
			return
		}

		provided := authHeader[len(prefix):]

		// Constant-time comparison to prevent timing attacks (OpenClaw: timingSafeEqual).
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "invalid bearer token",
					"type":    "authentication_error",
				},
			})
			return
		}

		c.Next()
	}
}

// isLocalRequest checks if a request originates from loopback address.
// Aligned with OpenClaw's isLocalDirectRequest check.
func isLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}
