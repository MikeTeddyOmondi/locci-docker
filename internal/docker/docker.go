// Package docker is a thin wrapper over the Docker Engine API.
// Handlers, jobs and the CLI all go through it so behaviour stays identical.
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

// NetworkSpec is the request body for creating a network.
type NetworkSpec struct {
	Name   string            `json:"name" binding:"required"`
	Driver string            `json:"driver"`
	Labels map[string]string `json:"labels"`
}

// ContainerSpec is the request body for creating a container.
type ContainerSpec struct {
	Name        string            `json:"name" binding:"required"`
	Image       string            `json:"image" binding:"required"`
	Ports       map[string]string `json:"ports"` // container port -> host port
	Environment []string          `json:"environment"`
	Networks    []string          `json:"networks"`
	Subdomain   string            `json:"subdomain" binding:"required"`
	Labels      map[string]string `json:"labels"`
	Volumes     []string          `json:"volumes"`
	Command     []string          `json:"command"`
}

// Created is returned after a container is created.
type Created struct {
	ID       string   `json:"container_id"`
	Name     string   `json:"name"`
	Warnings []string `json:"warnings"`
	Domain   string   `json:"domain"`
}

type Service struct {
	Client *client.Client
	Domain string // Traefik base domain, e.g. locci.cloud
	Log    *slog.Logger

	ssh     *sshDialer   // set for ssh:// hosts (see connect.go)
	dialErr func() error // the real cause of the last failed dial, for custom dialers
}

// New connects to host (or DOCKER_HOST / the local socket when empty).
// Remote hosts with TLS or SSH credentials use NewWithOptions.
func New(host, domain string, log *slog.Logger) (*Service, error) {
	return NewWithOptions(Options{Host: host, Domain: domain, Log: log})
}

// Ping checks the daemon is reachable. With a custom dialer (SSH, agent
// tunnel) the Docker client hides the dial error behind "Cannot connect to
// the Docker daemon at http://docker.invalid", so the real cause is returned.
func (s *Service) Ping(ctx context.Context) error {
	_, err := s.Client.Ping(ctx)
	if err != nil && s.dialErr != nil {
		if cause := s.dialErr(); cause != nil {
			return cause
		}
	}
	return err
}

// ---- networks ----

func (s *Service) CreateNetwork(ctx context.Context, n NetworkSpec) (network.CreateResponse, error) {
	if n.Driver == "" {
		n.Driver = "bridge"
	}
	return s.Client.NetworkCreate(ctx, n.Name, network.CreateOptions{Driver: n.Driver, Labels: n.Labels})
}

func (s *Service) ListNetworks(ctx context.Context) ([]network.Summary, error) {
	return s.Client.NetworkList(ctx, network.ListOptions{})
}

// ---- containers ----

// TraefikLabels routes <subdomain>.<domain> to the given container port.
func (s *Service) TraefikLabels(subdomain, port string) map[string]string {
	return map[string]string{
		"traefik.enable": "true",
		"traefik.http.routers." + subdomain + ".rule":                      fmt.Sprintf("Host(`%s.%s`)", subdomain, s.Domain),
		"traefik.http.routers." + subdomain + ".tls":                       "true",
		"traefik.http.routers." + subdomain + ".tls.certresolver":          "letsencrypt",
		"traefik.http.services." + subdomain + ".loadbalancer.server.port": port,
	}
}

// CreateContainer pulls the image, then creates (but does not start) the container.
func (s *Service) CreateContainer(ctx context.Context, spec ContainerSpec) (Created, error) {
	if err := CheckSubdomain(spec.Subdomain); err != nil {
		return Created{}, err
	}
	exposed := nat.PortSet{}
	bindings := nat.PortMap{}
	mainPort := ""

	for cport, hport := range spec.Ports {
		p, err := nat.NewPort("tcp", cport)
		if err != nil {
			return Created{}, fmt.Errorf("invalid container port %q: %w", cport, err)
		}
		exposed[p] = struct{}{}
		bindings[p] = []nat.PortBinding{{HostPort: hport}}
		if mainPort == "" {
			mainPort = strings.Split(cport, "/")[0]
		}
	}
	if mainPort == "" {
		mainPort = "80"
	}

	labels := s.TraefikLabels(spec.Subdomain, mainPort)
	maps.Copy(labels, spec.Labels)

	netCfg := &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{}}
	for _, n := range spec.Networks {
		netCfg.EndpointsConfig[n] = &network.EndpointSettings{}
	}

	// Always try to pull so tags like :latest are fresh, but fall back to a
	// local copy (e.g. an image built on this host) if the pull fails.
	if err := s.Pull(ctx, spec.Image); err != nil {
		if _, ierr := s.Client.ImageInspect(ctx, spec.Image); ierr != nil {
			return Created{}, err
		}
		s.Log.Warn("pull failed, using local image", "image", spec.Image, "err", err)
	}

	resp, err := s.Client.ContainerCreate(ctx,
		&container.Config{
			Image:        spec.Image,
			Env:          spec.Environment,
			ExposedPorts: exposed,
			Labels:       labels,
			Cmd:          spec.Command,
		},
		&container.HostConfig{PortBindings: bindings, Binds: spec.Volumes},
		netCfg, nil, spec.Name)
	if err != nil {
		return Created{}, err
	}

	return Created{
		ID:       resp.ID,
		Name:     spec.Name,
		Warnings: resp.Warnings,
		Domain:   spec.Subdomain + "." + s.Domain,
	}, nil
}

func (s *Service) StartContainer(ctx context.Context, id string) error {
	return s.Client.ContainerStart(ctx, id, container.StartOptions{})
}

// RemoveContainer stops (when force) and removes a container.
func (s *Service) RemoveContainer(ctx context.Context, id string, force bool) error {
	if force {
		_ = s.Client.ContainerStop(ctx, id, container.StopOptions{})
	}
	return s.Client.ContainerRemove(ctx, id, container.RemoveOptions{Force: force})
}

func (s *Service) ListContainers(ctx context.Context) ([]container.Summary, error) {
	return s.Client.ContainerList(ctx, container.ListOptions{All: true})
}

// ---- images ----

// Pull pulls an image and waits for it to finish.
func (s *Service) Pull(ctx context.Context, ref string) error {
	body, err := s.Client.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull %s: %w", ref, err)
	}
	defer body.Close()

	dec := json.NewDecoder(body)
	for dec.More() {
		var msg struct {
			Error string `json:"error"`
		}
		if err := dec.Decode(&msg); err != nil {
			return fmt.Errorf("pull %s: %w", ref, err)
		}
		if msg.Error != "" {
			return fmt.Errorf("pull %s: %s", ref, msg.Error)
		}
	}

	s.Log.Info("pulled image", "image", ref)
	return nil
}
