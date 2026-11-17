package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/MikeTeddyOmondi/locci-docker/internal/docker"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
)

// useEnv resolves the environment for a request and puts its Docker service on
// the context. Routes under /environments/:env use that environment; the
// original routes (no :env) use the default one, so they behave as before.
func (s *Server) useEnv(c *gin.Context) {
	var (
		svc *docker.Service
		e   store.Environment
		err error
	)
	if name := c.Param("env"); name != "" {
		svc, e, err = s.Envs.Get(name)
	} else {
		svc, e, err = s.Envs.Default()
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		s.fail(c, http.StatusNotFound, fmt.Errorf("environment %q not found", c.Param("env")))
		return
	case err != nil:
		// Bad stored settings or a missing LOCCI_SECRET_KEY: the host can't be reached.
		s.fail(c, http.StatusServiceUnavailable, fmt.Errorf("environment %q: %w", e.Name, err))
		return
	}
	c.Set("docker", svc)
	c.Set("env", e)
	c.Next()
}

// dk returns the Docker service chosen by useEnv.
func dk(c *gin.Context) *docker.Service {
	return c.MustGet("docker").(*docker.Service)
}
