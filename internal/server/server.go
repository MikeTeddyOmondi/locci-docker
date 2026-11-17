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
	"github.com/MikeTeddyOmondi/locci-docker/internal/env"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
	"github.com/MikeTeddyOmondi/locci-docker/internal/ui"
)

type Server struct {
	Envs      *env.Registry // one Docker connection per environment
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

	// Docker resources on the default environment. The four legacy container
	// routes and GET/POST /networks are the original v1 API, unchanged.
	def := api.Group("", s.useEnv)
	def.POST("/containers/start", s.startContainerLegacy)
	def.DELETE("/containers", s.deleteContainerLegacy)
	s.resourceRoutes(def)

	// The same routes for a chosen environment: /api/v1/environments/:env/...
	scoped := api.Group("/environments/:env", s.useEnv)
	scoped.GET("/info", s.systemInfo)
	s.resourceRoutes(scoped)

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

// resourceRoutes registers every Docker resource route on g, which must run useEnv.
func (s *Server) resourceRoutes(g *gin.RouterGroup) {
	g.GET("/system/info", s.systemInfo)

	// Containers
	g.GET("/containers", s.listContainers)
	g.POST("/containers", s.createContainer)
	g.GET("/containers/:id", s.inspectContainer)
	g.DELETE("/containers/:id", s.removeContainer)
	for _, action := range docker.Actions {
		g.POST("/containers/:id/"+action, s.containerAction(action))
	}
	g.GET("/containers/:id/logs", s.containerLogs)
	g.GET("/containers/:id/stats", s.containerStats)
	g.POST("/containers/:id/exec", s.containerExec)
	g.GET("/containers/:id/exec/ws", s.execWS)

	// Images
	g.GET("/images", s.listImages)
	g.POST("/images/pull", s.pullImage)
	g.POST("/images/prune", s.pruneImages)
	g.DELETE("/images/:id", s.removeImage)

	// Networks
	g.GET("/networks", s.listNetworks)
	g.POST("/networks", s.createNetwork)
	g.GET("/networks/:id", s.inspectNetwork)
	g.DELETE("/networks/:id", s.removeNetwork)

	// Volumes
	g.GET("/volumes", s.listVolumes)
	g.POST("/volumes", s.createVolume)
	g.DELETE("/volumes/:name", s.removeVolume)
}

// logRequests logs method, path (never the query, which may hold a token) and status.
func (s *Server) logRequests(c *gin.Context) {
	start := time.Now()
	c.Next()
	s.Log.Info("http", "method", c.Request.Method, "path", c.Request.URL.Path,
		"status", c.Writer.Status(), "ms", time.Since(start).Milliseconds())
}

// health reports whether the default environment's Docker is reachable.
func (s *Server) health(c *gin.Context) {
	svc, _, err := s.Envs.Default()
	if err == nil {
		err = svc.Ping(c.Request.Context())
	}
	if err != nil {
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
