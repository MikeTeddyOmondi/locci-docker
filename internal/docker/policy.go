package docker

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
)

// ProtectedLabel marks a container that only admins may change or open a
// console in. The locci image sets it, so the server and its agents carry it.
const ProtectedLabel = "io.locci.protected"

// subdomainRE is one DNS label: it becomes both a hostname and part of
// Traefik's label keys, where a dot or a backtick would change their meaning.
var subdomainRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// CheckSubdomain validates a container's subdomain.
func CheckSubdomain(s string) error {
	if !subdomainRE.MatchString(s) {
		return fmt.Errorf("subdomain %q: use lowercase letters, digits and dashes (one DNS label, no dots)", s)
	}
	return nil
}

// ProtectedReason says why a container gives host-level access (so only
// admins may touch it), or "" when it doesn't:
//   - it carries ProtectedLabel (the locci server and agents),
//   - it is privileged,
//   - it has the Docker socket or the host's root directory bind-mounted
//     (Traefik, Portainer, CI runners: a console there controls the host).
func ProtectedReason(info container.InspectResponse) string {
	if info.Config != nil && info.Config.Labels[ProtectedLabel] == "true" {
		return "it is labelled " + ProtectedLabel
	}
	if info.HostConfig != nil && info.HostConfig.Privileged {
		return "it is privileged"
	}
	return mountReason(info.Mounts)
}

// mountReason checks for a bind mount of the Docker socket or the host's /.
func mountReason(mounts []container.MountPoint) string {
	for _, m := range mounts {
		if m.Type == mount.TypeBind && (m.Source == "/" || strings.HasSuffix(m.Source, "docker.sock") || strings.HasSuffix(m.Destination, "docker.sock")) {
			return "it has the host's Docker socket or root directory mounted"
		}
	}
	return ""
}

// ProtectedSummaryReason is ProtectedReason for a container list entry, which
// carries labels and mounts but not the privileged flag (the per-container
// routes still check that).
func ProtectedSummaryReason(c container.Summary) string {
	if c.Labels[ProtectedLabel] == "true" {
		return "it is labelled " + ProtectedLabel
	}
	return mountReason(c.Mounts)
}

// RouterOwner returns the name of a container that already routes subdomain
// through Traefik, or "".
func (s *Service) RouterOwner(ctx context.Context, subdomain string) (string, error) {
	list, err := s.Client.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", "traefik.http.routers."+subdomain+".rule")),
	})
	if err != nil || len(list) == 0 {
		return "", err
	}
	if len(list[0].Names) > 0 {
		return strings.TrimPrefix(list[0].Names[0], "/"), nil
	}
	return list[0].ID[:12], nil
}
