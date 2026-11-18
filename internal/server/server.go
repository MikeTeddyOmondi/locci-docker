// Package server exposes the HTTP API and serves the embedded web UI.
package server

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/gin-gonic/gin"

	"github.com/MikeTeddyOmondi/locci-docker/internal/docker"
	"github.com/MikeTeddyOmondi/locci-docker/internal/env"
	"github.com/MikeTeddyOmondi/locci-docker/internal/sso"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
	"github.com/MikeTeddyOmondi/locci-docker/internal/ui"
)

type Server struct {
	Envs      *env.Registry // one Docker connection per environment
	SSO       *sso.Service  // OIDC sign-in; inactive until a provider is enabled
	PublicURL string        // LOCCI_PUBLIC_URL; empty = derived from each request
	Store     *store.Store
	AuthToken string // optional static admin token (AUTH_TOKEN)
	Log       *slog.Logger

	// TrustedProxies (LOCCI_TRUSTED_PROXIES, see ParseProxies) may set
	// X-Forwarded-For/-Proto/-Host. nil means DefaultProxies.
	TrustedProxies []string

	proxyOnce sync.Once
	proxyNets []*net.IPNet
}

// Router builds the gin engine with every route.
func (s *Server) Router(mode string) *gin.Engine {
	gin.SetMode(mode)
	r := gin.New()
	// c.ClientIP() believes X-Forwarded-For only from trusted proxies.
	nets := s.proxies()
	cidrs := make([]string, len(nets))
	for i, n := range nets {
		cidrs[i] = n.String()
	}
	_ = r.SetTrustedProxies(cidrs)
	r.Use(s.logRequests, gin.Recovery(), s.securityHeaders(), limitBody)

	r.GET("/health", s.health)
	r.POST("/api/v1/auth/login", s.login)

	// Single sign-on. These answer 404 for providers that aren't enabled, so
	// with none set up only password login exists.
	r.GET("/api/v1/auth/providers", s.authProviders)
	r.GET("/api/v1/auth/oidc/:provider/start", s.ssoStart)
	r.GET("/api/v1/auth/oidc/:provider/callback", s.ssoCallback)
	r.POST("/api/v1/auth/oidc/:provider/device", s.ssoDeviceStart)
	r.POST("/api/v1/auth/oidc/:provider/device/token", s.ssoDevicePoll)

	// Outbound agents authenticate with their own token, not a user's.
	r.GET("/api/v1/agent/connect", s.agentConnect)

	api := r.Group("/api/v1", s.auth, s.audit)

	api.GET("/auth/me", s.me)
	api.POST("/auth/logout", s.logout)

	// Docker resources on the default environment. The four legacy container
	// routes and GET/POST /networks are the original v1 API, unchanged.
	def := api.Group("", s.useEnv)
	def.POST("/containers/start", s.startContainerLegacy)
	def.DELETE("/containers", s.deleteContainerLegacy)
	s.resourceRoutes(def)

	// Environments: anyone signed in can list them (for the switcher); only
	// admins can change them. Secrets are write-only.
	api.GET("/environments", s.listEnvironments)
	api.GET("/environments/:env", s.getEnvironment)
	envAdmin := api.Group("/environments", adminOnly)
	envAdmin.POST("", s.createEnvironment)
	envAdmin.POST("/test", s.testEnvironment)
	envAdmin.PATCH("/:env", s.updateEnvironment)
	envAdmin.DELETE("/:env", s.deleteEnvironment)
	envAdmin.POST("/:env/default", s.setDefaultEnvironment)

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
	admin.GET("/sso/providers", s.listProviders)
	admin.POST("/sso/providers", s.createProvider)
	admin.POST("/sso/providers/test", s.testProvider)
	admin.PATCH("/sso/providers/:provider", s.updateProvider)
	admin.DELETE("/sso/providers/:provider", s.deleteProvider)
	admin.GET("/sso/settings", s.getSSOSettings)
	admin.PUT("/sso/settings", s.putSSOSettings)

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
	if bodyTooLarge(err) {
		code = http.StatusRequestEntityTooLarge
	}
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
