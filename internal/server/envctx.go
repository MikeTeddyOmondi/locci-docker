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
		// Check access before connecting; an environment the user may not
		// use looks the same as one that doesn't exist.
		if stored, serr := s.Store.EnvironmentByName(name); serr == nil && !s.canUse(c, stored) {
			s.fail(c, http.StatusNotFound, fmt.Errorf("environment %q not found", name))
			return
		}
		svc, e, err = s.Envs.Get(name)
	} else {
		svc, e, err = s.Envs.Default()
		if err == nil && !s.canUse(c, e) {
			s.fail(c, http.StatusForbidden, fmt.Errorf("you don't have access to the default environment %q; pick one under /environments", e.Name))
			return
		}
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

// canUse reports whether the caller may use environment e: admins always,
// others when e has no access list or lists them.
func (s *Server) canUse(c *gin.Context, e store.Environment) bool {
	u := currentUser(c)
	if u.IsAdmin() {
		return true
	}
	ok, err := s.Store.CanUseEnvironment(e.ID, u.ID)
	if err != nil {
		s.Log.Error("environment access check failed", "env", e.Name, "err", err)
		return false
	}
	return ok
}

// dk returns the Docker service chosen by useEnv.
func dk(c *gin.Context) *docker.Service {
	return c.MustGet("docker").(*docker.Service)
}
