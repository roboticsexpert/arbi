// Package httpx holds HTTP concerns shared by the API: CORS for the dashboard
// origin and the token gate that protects the data endpoints.
package httpx

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthHeader is the header the dashboard sends its token in.
const AuthHeader = "X-Dashboard-Token"

// MT5Header is the header the MetaTrader Expert Advisor sends its token in.
const MT5Header = "X-MT5-Token"

// CORS allows the dashboard origin(s) to call the API from the browser.
// allowedOrigins is a comma-separated list; "*" allows any origin.
func CORS(allowedOrigins string) gin.HandlerFunc {
	origins := map[string]bool{}
	allowAll := false
	for _, o := range strings.Split(allowedOrigins, ",") {
		o = strings.TrimSpace(strings.TrimSuffix(o, "/"))
		if o == "" {
			continue
		}
		if o == "*" {
			allowAll = true
		}
		origins[o] = true
	}

	return func(c *gin.Context) {
		origin := strings.TrimSuffix(c.Request.Header.Get("Origin"), "/")
		if origin != "" && (allowAll || origins[origin]) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Headers", "Content-Type, "+AuthHeader)
			c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
			c.Header("Access-Control-Max-Age", "600")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// RequireToken rejects requests that do not carry the dashboard token.
// When token is empty the gate is disabled and every request passes through,
// so an unconfigured deployment behaves exactly as it did before.
func RequireToken(token string) gin.HandlerFunc {
	return RequireTokenHeader(AuthHeader, token)
}

// RequireTokenHeader is RequireToken against an arbitrary header, so a writer
// like the MetaTrader feed can carry its own credential instead of borrowing
// the dashboard's read token.
func RequireTokenHeader(header, token string) gin.HandlerFunc {
	if token == "" {
		return func(c *gin.Context) { c.Next() }
	}

	want := []byte(token)
	return func(c *gin.Context) {
		got := c.GetHeader(header)
		if got == "" {
			// Allow ?token= as a fallback so the endpoints stay usable from a
			// plain browser tab or curl.
			got = c.Query("token")
		}
		if subtle.ConstantTimeCompare([]byte(got), want) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}
