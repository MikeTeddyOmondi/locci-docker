package server

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/MikeTeddyOmondi/locci-docker/internal/ui"
)

// MaxBody caps request bodies. The largest legitimate ones are environments
// with PEM certificates and keys, a few kilobytes each.
const MaxBody = 1 << 20

// HTTPServer wraps the router with timeouts. There is no overall read or
// write timeout: log and stats streams, image pulls, the console and agent
// tunnels stay open for as long as they're in use. ReadHeaderTimeout stops
// clients that open connections and never finish sending a request.
func (s *Server) HTTPServer(addr, mode string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           s.Router(mode),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
	}
}

// limitBody rejects request bodies over MaxBody (see bodyTooLarge).
func limitBody(c *gin.Context) {
	if c.Request.Body != nil {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxBody)
	}
	c.Next()
}

// bodyTooLarge reports whether err came from limitBody.
func bodyTooLarge(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

// securityHeaders sets the browser protections on every response. The
// Content-Security-Policy allows only this server's own scripts (and the one
// inline SvelteKit bootstrap script, by hash), styles, fonts and images, and
// forbids framing the UI on other sites.
func (s *Server) securityHeaders() gin.HandlerFunc {
	csp := "default-src 'self'; " +
		"script-src 'self'" + ui.InlineScriptHashes() + "; " +
		// bits-ui, transitions and the terminal set style attributes.
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; font-src 'self' data:; " +
		"connect-src 'self'; " +
		"object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		if s.secureCookie(c) {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		c.Next()
	}
}
