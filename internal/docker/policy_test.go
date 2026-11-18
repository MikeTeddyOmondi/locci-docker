package docker

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
)

func TestCheckSubdomain(t *testing.T) {
	for _, ok := range []string{"web", "my-app", "a1", "x"} {
		if err := CheckSubdomain(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "a.b", "Web", "-a", "a-", "a`) || Host(`evil.com", "x.rule"} {
		if CheckSubdomain(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestProtectedReason(t *testing.T) {
	plain := container.InspectResponse{Config: &container.Config{}, ContainerJSONBase: &container.ContainerJSONBase{HostConfig: &container.HostConfig{}}}
	if r := ProtectedReason(plain); r != "" {
		t.Errorf("plain container: %q", r)
	}
	labelled := plain
	labelled.Config = &container.Config{Labels: map[string]string{ProtectedLabel: "true"}}
	privileged := plain
	privileged.ContainerJSONBase = &container.ContainerJSONBase{HostConfig: &container.HostConfig{Privileged: true}}
	socket := plain
	socket.Mounts = []container.MountPoint{{Type: mount.TypeBind, Source: "/var/run/docker.sock", Destination: "/var/run/docker.sock"}}
	root := plain
	root.Mounts = []container.MountPoint{{Type: mount.TypeBind, Source: "/", Destination: "/host"}}
	for name, c := range map[string]container.InspectResponse{"label": labelled, "privileged": privileged, "socket": socket, "root": root} {
		if ProtectedReason(c) == "" {
			t.Errorf("%s: not protected", name)
		}
	}
	vol := plain
	vol.Mounts = []container.MountPoint{{Type: mount.TypeVolume, Name: "data", Destination: "/data"}}
	if ProtectedReason(vol) != "" {
		t.Error("named volume counted as host access")
	}
}
