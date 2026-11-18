package server

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/MikeTeddyOmondi/locci-docker/internal/docker"
)

// Plain users (role "user") run containers but must not get the hosts: no
// host paths in containers, no Traefik or locci labels, no taking over another
// container's subdomain, and no console in or changes to containers that
// control the host (docker.ProtectedReason). Admins are not limited, and
// LOCCI_USER_HOST_ACCESS=true lifts the limits for everyone.

// restricted reports whether the caller is held to the plain-user limits.
func (s *Server) restricted(c *gin.Context) bool {
	return !s.UserHostAccess && !currentUser(c).IsAdmin()
}

// namedVolume is a Docker volume name, as opposed to a host path.
var namedVolume = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// reservedLabels may only be set by admins: Traefik's decide where traffic
// goes, locci's mark protected containers.
var reservedLabels = []string{"traefik.", "io.locci."}

// checkSpec applies the plain-user limits to a new container.
func (s *Server) checkSpec(c *gin.Context, spec docker.ContainerSpec) (int, error) {
	if !s.restricted(c) {
		return 0, nil
	}
	for _, v := range spec.Volumes {
		src, _, hasTarget := strings.Cut(v, ":")
		if hasTarget && !namedVolume.MatchString(src) {
			return http.StatusForbidden, fmt.Errorf("volume %q: only admins can mount host paths; use a named volume such as data:%s", v, strings.SplitN(v, ":", 3)[1])
		}
	}
	for k := range spec.Labels {
		for _, p := range reservedLabels {
			if strings.HasPrefix(k, p) {
				return http.StatusForbidden, fmt.Errorf("label %q: only admins can set %s* labels", k, p)
			}
		}
	}
	owner, err := dk(c).RouterOwner(c.Request.Context(), spec.Subdomain)
	if err != nil {
		return statusFor(err), err
	}
	if owner != "" {
		return http.StatusConflict, fmt.Errorf("subdomain %q is already used by container %s", spec.Subdomain, owner)
	}
	return 0, nil
}

// errProtected is returned for a change to a protected container.
var errProtected = errors.New("only admins can change or open a console in this container")

// allowContainer reports whether the caller may change container id (start,
// stop, remove, exec, console). It answers the request itself when not.
func (s *Server) allowContainer(c *gin.Context, id string) bool {
	if !s.restricted(c) {
		return true
	}
	info, err := dk(c).InspectContainer(c.Request.Context(), id)
	if err != nil {
		s.fail(c, statusFor(err), err)
		return false
	}
	if why := docker.ProtectedReason(info); why != "" {
		s.fail(c, http.StatusForbidden, fmt.Errorf("%w: %s", errProtected, why))
		return false
	}
	return true
}

// guardContainer is allowContainer for routes with :id.
func (s *Server) guardContainer(c *gin.Context) {
	if s.allowContainer(c, c.Param("id")) {
		c.Next()
	}
}
