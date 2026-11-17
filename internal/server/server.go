// Package server exposes the HTTP API (and later the embedded UI).
package server

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/MikeTeddyOmondi/locci-docker/internal/docker"
)

type Server struct {
	Docker    *docker.Service
	AuthToken string
	Log       *slog.Logger
}

// Router builds the gin engine with every route.
func (s *Server) Router(mode string) *gin.Engine {
	gin.SetMode(mode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/health", s.health)

	api := r.Group("/api/v1", s.auth)

	// v1 routes kept exactly as before.
	api.POST("/networks", s.createNetwork)
	api.GET("/networks", s.listNetworks)
	api.POST("/containers", s.createContainer)
	api.POST("/containers/start", s.startContainerLegacy)
	api.DELETE("/containers", s.deleteContainerLegacy)
	api.GET("/containers", s.listContainers)

	return r
}

// auth checks "Authorization: Bearer <token>".
func (s *Server) auth(c *gin.Context) {
	token, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if !ok || token == "" || token != s.AuthToken {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or missing bearer token"})
		return
	}
	c.Next()
}

func (s *Server) health(c *gin.Context) {
	if err := s.Docker.Ping(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "healthy", "docker": "connected"})
}

// fail writes an error response and logs it.
func (s *Server) fail(c *gin.Context, code int, err error) {
	if code >= 500 {
		s.Log.Error("request failed", "path", c.FullPath(), "err", err)
	}
	c.AbortWithStatusJSON(code, gin.H{"error": err.Error()})
}
