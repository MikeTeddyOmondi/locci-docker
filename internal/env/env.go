// Package env holds one Docker connection per environment (Docker host).
//
// Connections are made lazily on first use and cached. Each cached connection
// remembers a signature of the settings it was built from, so editing an
// environment rebuilds its client on the next request without explicit
// invalidation. A background loop (Run) pings every environment and records
// up/down in the store for the UI.
package env

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/MikeTeddyOmondi/locci-docker/internal/docker"
	"github.com/MikeTeddyOmondi/locci-docker/internal/secret"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
)

// HealthTimeout bounds a single ping in the health loop and in Test.
const HealthTimeout = 8 * time.Second

type Registry struct {
	Store      *store.Store
	Box        *secret.Box // may be nil: only local environments work then
	DockerHost string      // DOCKER_HOST, used by local environments with no URL
	Domain     string      // Traefik domain passed to every docker.Service
	Log        *slog.Logger

	mu    sync.Mutex
	conns map[int64]conn
}

type conn struct {
	svc *docker.Service
	sig string
}

// Options turns a stored environment into connection options, decrypting its secret.
func (r *Registry) Options(e store.Environment) (docker.Options, error) {
	key := ""
	if e.Kind == store.KindTLS || e.Kind == store.KindSSH {
		var err error
		if key, err = r.Box.Open(e.Secret); err != nil {
			return docker.Options{}, err
		}
	}
	return r.PlainOptions(e, key)
}

// PlainOptions builds connection options from e and an already-decrypted key
// (the TLS client key or SSH private key). Used directly to test unsaved hosts.
func (r *Registry) PlainOptions(e store.Environment, key string) (docker.Options, error) {
	o := docker.Options{Host: e.URL, Domain: r.Domain, Log: r.Log}
	switch e.Kind {
	case store.KindLocal:
		if o.Host == "" {
			o.Host = r.DockerHost
		}
	case store.KindTLS:
		o.TLS = &docker.TLSOptions{CA: e.TLSCA, Cert: e.TLSCert, Key: key}
	case store.KindSSH:
		o.SSH = &docker.SSHOptions{PrivateKey: key, HostKey: e.SSHHostKey}
	default:
		return o, errors.New("unknown environment kind " + e.Kind)
	}
	return o, nil
}

// signature changes whenever anything that affects the connection changes.
func signature(e store.Environment) string {
	h := sha256.New()
	for _, s := range []string{e.Kind, e.URL, e.TLSCA, e.TLSCert, e.Secret, e.SSHHostKey} {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Get returns the Docker service for the named environment.
func (r *Registry) Get(name string) (*docker.Service, store.Environment, error) {
	e, err := r.Store.EnvironmentByName(name)
	if err != nil {
		return nil, e, err
	}
	svc, err := r.service(e)
	return svc, e, err
}

// Default returns the Docker service for the default environment.
func (r *Registry) Default() (*docker.Service, store.Environment, error) {
	e, err := r.Store.DefaultEnvironment()
	if err != nil {
		return nil, e, err
	}
	svc, err := r.service(e)
	return svc, e, err
}

func (r *Registry) service(e store.Environment) (*docker.Service, error) {
	sig := signature(e)

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conns == nil {
		r.conns = map[int64]conn{}
	}
	if c, ok := r.conns[e.ID]; ok {
		if c.sig == sig {
			return c.svc, nil
		}
		c.svc.Close() // settings changed: rebuild
	}

	o, err := r.Options(e)
	if err != nil {
		return nil, err
	}
	svc, err := docker.NewWithOptions(o)
	if err != nil {
		return nil, err
	}
	r.conns[e.ID] = conn{svc: svc, sig: sig}
	return svc, nil
}

// Forget closes and drops the cached connection for an environment.
func (r *Registry) Forget(id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.conns[id]; ok {
		c.svc.Close()
		delete(r.conns, id)
	}
}

// Check pings an environment, records up/down, and pins an SSH host key the
// first time one is seen. A failed check drops the cached connection so the
// next request reconnects from scratch.
func (r *Registry) Check(ctx context.Context, e store.Environment) error {
	svc, err := r.service(e)
	if err == nil {
		pctx, cancel := context.WithTimeout(ctx, HealthTimeout)
		err = svc.Ping(pctx)
		cancel()
	}

	status := store.StatusUp
	if err != nil {
		status = store.StatusDown
		r.Forget(e.ID)
	} else if e.Kind == store.KindSSH && e.SSHHostKey == "" {
		if key := svc.SSHHostKey(); key != "" {
			e.SSHHostKey = key
			if _, uerr := r.Store.UpdateEnvironment(e); uerr != nil {
				r.Log.Warn("pin ssh host key", "env", e.Name, "err", uerr)
			}
		}
	}
	if serr := r.Store.SetEnvironmentStatus(e.ID, status); serr != nil {
		r.Log.Warn("record environment status", "env", e.Name, "err", serr)
	}
	return err
}

// CheckAll checks every environment concurrently.
func (r *Registry) CheckAll(ctx context.Context) {
	envs, err := r.Store.ListEnvironments()
	if err != nil {
		r.Log.Warn("list environments", "err", err)
		return
	}
	var wg sync.WaitGroup
	for _, e := range envs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.Check(ctx, e); err != nil {
				r.Log.Debug("environment down", "env", e.Name, "err", err)
			}
		}()
	}
	wg.Wait()
}

// Run checks every environment now and then every interval until ctx ends.
func (r *Registry) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		r.CheckAll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// TestResult describes a host reached by Test.
type TestResult struct {
	Name          string `json:"name"`
	ServerVersion string `json:"server_version"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	CPUs          int    `json:"cpus"`
	Memory        int64  `json:"memory"`
	Containers    int    `json:"containers"`
	SSHHostKey    string `json:"ssh_host_key,omitempty"`
	Fingerprint   string `json:"ssh_fingerprint,omitempty"` // SHA256:... for people to compare
}

// Test connects to an unsaved environment once, without caching or storing
// anything. For SSH it also returns the host key the server presented, so it
// can be shown, compared and pinned when the environment is saved.
func Test(ctx context.Context, o docker.Options) (TestResult, error) {
	svc, err := docker.NewWithOptions(o)
	if err != nil {
		return TestResult{}, err
	}
	defer svc.Close()
	ctx, cancel := context.WithTimeout(ctx, HealthTimeout)
	defer cancel()
	if err := svc.Ping(ctx); err != nil {
		return TestResult{}, err
	}
	res := TestResult{SSHHostKey: svc.SSHHostKey(), Fingerprint: Fingerprint(svc.SSHHostKey())}
	if info, err := svc.Info(ctx); err == nil {
		res.Name, res.ServerVersion, res.OS, res.Arch = info.Name, info.ServerVersion, info.OperatingSystem, info.Architecture
		res.CPUs, res.Memory, res.Containers = info.NCPU, info.MemTotal, info.Containers
	}
	return res, nil
}

// Fingerprint turns an authorized_keys line into "SHA256:...", or "" if empty/invalid.
func Fingerprint(authorizedKey string) string {
	if authorizedKey == "" {
		return ""
	}
	k, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authorizedKey))
	if err != nil {
		return ""
	}
	return ssh.FingerprintSHA256(k)
}

// Close drops every cached connection.
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, c := range r.conns {
		c.svc.Close()
		delete(r.conns, id)
	}
}
