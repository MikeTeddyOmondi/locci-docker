// Package server exposes the HTTP API and serves the embedded web UI.
package server

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/gin-gonic/gin"

	"github.com/MikeTeddyOmondi/locci-docker/internal/docker"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
	"github.com/MikeTeddyOmondi/locci-docker/internal/ui"
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
	r.Use(s.logRequests, gin.Recovery())

	r.GET("/health", s.health)
	r.POST("/api/v1/auth/login", s.login)

	api := r.Group("/api/v1", s.auth, s.audit)

	api.GET("/auth/me", s.me)
	api.POST("/auth/logout", s.logout)
	api.GET("/system/info", s.systemInfo)

	// Containers. The first four routes are the original v1 API, unchanged.
	api.GET("/containers", s.listContainers)
	api.POST("/containers", s.createContainer)
	api.POST("/containers/start", s.startContainerLegacy)
	api.DELETE("/containers", s.deleteContainerLegacy)
	api.GET("/containers/:id", s.inspectContainer)
	api.DELETE("/containers/:id", s.removeContainer)
	for _, action := range docker.Actions {
		api.POST("/containers/:id/"+action, s.containerAction(action))
	}
	api.GET("/containers/:id/logs", s.containerLogs)
	api.GET("/containers/:id/stats", s.containerStats)
	api.POST("/containers/:id/exec", s.containerExec)
	api.GET("/containers/:id/exec/ws", s.execWS)

	// Images
	api.GET("/images", s.listImages)
	api.POST("/images/pull", s.pullImage)
	api.POST("/images/prune", s.pruneImages)
	api.DELETE("/images/:id", s.removeImage)

	// Networks (first two are original v1)
	api.GET("/networks", s.listNetworks)
	api.POST("/networks", s.createNetwork)
	api.GET("/networks/:id", s.inspectNetwork)
	api.DELETE("/networks/:id", s.removeNetwork)

	// Volumes
	api.GET("/volumes", s.listVolumes)
	api.POST("/volumes", s.createVolume)
	api.DELETE("/volumes/:name", s.removeVolume)

	// Admin
	admin := api.Group("", adminOnly)
	admin.GET("/users", s.listUsers)
	admin.POST("/users", s.createUser)
	admin.DELETE("/users/:name", s.deleteUser)
	admin.GET("/audit", s.listAudit)

	// Everything else is the embedded web UI (unknown /api paths stay JSON 404s).
	webUI := ui.Handler()
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		webUI.ServeHTTP(c.Writer, c.Request)
	})

	return r
}

// logRequests logs method, path (never the query, which may hold a token) and status.
func (s *Server) logRequests(c *gin.Context) {
	start := time.Now()
	c.Next()
	s.Log.Info("http", "method", c.Request.Method, "path", c.Request.URL.Path,
		"status", c.Writer.Status(), "ms", time.Since(start).Milliseconds())
}

func (s *Server) health(c *gin.Context) {
	if err := s.Docker.Ping(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "healthy", "docker": "connected"})
}

// fail writes an error response and logs server-side errors.
func (s *Server) fail(c *gin.Context, code int, err error) {
	if code >= 500 {
		s.Log.Error("request failed", "path", c.FullPath(), "err", err)
	}
	c.AbortWithStatusJSON(code, gin.H{"error": err.Error()})
}

// statusFor maps Docker errors to HTTP status codes.
func statusFor(err error) int {
	switch {
	case cerrdefs.IsNotFound(err):
		return http.StatusNotFound
	case cerrdefs.IsConflict(err), cerrdefs.IsAlreadyExists(err):
		return http.StatusConflict
	case cerrdefs.IsInvalidArgument(err):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
