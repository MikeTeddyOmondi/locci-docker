// Package server exposes the HTTP API (and later the embedded UI).
package server

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/MikeTeddyOmondi/locci-docker/internal/docker"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
)

type Server struct {
	Docker    *docker.Service
	Store     *store.Store
	AuthToken string // optional static admin token (AUTH_TOKEN)
	Log       *slog.Logger
}

// Router builds the gin engine with every route.
func (s *Server) Router(mode string) *gin.Engine {
	gin.SetMode(mode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/health", s.health)
	r.POST("/api/v1/auth/login", s.login)

	api := r.Group("/api/v1", s.auth, s.audit)

	api.GET("/auth/me", s.me)
	api.POST("/auth/logout", s.logout)

	admin := api.Group("", adminOnly)
	admin.GET("/users", s.listUsers)
	admin.POST("/users", s.createUser)
	admin.DELETE("/users/:name", s.deleteUser)
	admin.GET("/audit", s.listAudit)

	// v1 routes kept exactly as before.
	api.POST("/networks", s.createNetwork)
	api.GET("/networks", s.listNetworks)
	api.POST("/containers", s.createContainer)
	api.POST("/containers/start", s.startContainerLegacy)
	api.DELETE("/containers", s.deleteContainerLegacy)
	api.GET("/containers", s.listContainers)

	return r
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
