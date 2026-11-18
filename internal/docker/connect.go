package docker

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/client"
	"golang.org/x/crypto/ssh"
)

// Options describes how to reach one Docker host.
type Options struct {
	Host   string // "" (DOCKER_HOST or local socket), unix://, tcp://, ssh://
	Domain string
	Log    *slog.Logger
	TLS    *TLSOptions // for tcp:// with client certificates
	SSH    *SSHOptions // for ssh://
	// Dial, when set, provides every connection to the Docker API (agent
	// environments dial through their tunnel). Host is ignored then.
	Dial func(ctx context.Context) (net.Conn, error)
}

// TLSOptions holds PEM-encoded material for Docker's TLS port.
type TLSOptions struct {
	CA, Cert, Key string
}

// SSHOptions holds an unencrypted private key (PEM) and the pinned host key in
// authorized_keys format. With no pinned key the first key seen is accepted
// and reported by Service.SSHHostKey so the caller can store it.
type SSHOptions struct {
	PrivateKey string
	HostKey    string
}

const defaultSocket = "/var/run/docker.sock"

// NewWithOptions connects to a local, tcp+tls or ssh Docker host.
// Connections are made lazily, so this only fails on bad configuration.
func NewWithOptions(o Options) (*Service, error) {
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.SSH != nil {
		return newSSH(o)
	}
	if o.Dial != nil {
		return newDialer(o)
	}

	opts := []client.Opt{client.WithAPIVersionNegotiation()}
	if o.TLS != nil {
		cfg, err := tlsConfig(*o.TLS)
		if err != nil {
			return nil, err
		}
		// The HTTP client must come before WithHost, which configures its transport.
		tr := &http.Transport{TLSClientConfig: cfg, TLSHandshakeTimeout: 10 * time.Second}
		opts = append(opts, client.WithHTTPClient(&http.Client{Transport: tr}))
	} else {
		opts = append(opts, client.FromEnv)
	}
	if o.Host != "" {
		opts = append(opts, client.WithHost(o.Host))
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &Service{Client: cli, Domain: o.Domain, Log: o.Log}, nil
}

// newDialer builds a client whose connections all come from o.Dial.
func newDialer(o Options) (*Service, error) {
	rec := &dialRecorder{}
	cli, err := client.NewClientWithOpts(
		client.WithHost("http://docker.invalid"),
		client.WithDialContext(func(ctx context.Context, _, _ string) (net.Conn, error) {
			c, err := o.Dial(ctx)
			rec.set(err)
			return c, err
		}),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &Service{Client: cli, Domain: o.Domain, Log: o.Log, dialErr: rec.get}, nil
}

// dialRecorder keeps the last dial error, which the Docker client would
// otherwise replace with a generic "Cannot connect" message.
type dialRecorder struct {
	mu  sync.Mutex
	err error
}

func (r *dialRecorder) set(err error) { r.mu.Lock(); r.err = err; r.mu.Unlock() }
func (r *dialRecorder) get() error    { r.mu.Lock(); defer r.mu.Unlock(); return r.err }

func tlsConfig(t TLSOptions) (*tls.Config, error) {
	if t.CA == "" || t.Cert == "" || t.Key == "" {
		return nil, errors.New("tcp+tls needs a CA certificate, a client certificate and a client key")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(t.CA)) {
		return nil, errors.New("CA certificate is not valid PEM")
	}
	pair, err := tls.X509KeyPair([]byte(t.Cert), []byte(t.Key))
	if err != nil {
		return nil, fmt.Errorf("client certificate/key: %w", err)
	}
	return &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}, nil
}

// sshDialer keeps one SSH connection and opens a stream to the remote Docker
// socket for every HTTP connection the Docker client makes. A broken SSH
// connection is dropped and redialled on the next request.
type sshDialer struct {
	addr, user, socket string
	config             *ssh.ClientConfig

	mu      sync.Mutex
	conn    *ssh.Client
	hostKey string // the key the server presented, authorized_keys format
	lastErr error  // the real cause of the last failed dial (see Service.Ping)
}

func newSSH(o Options) (*Service, error) {
	u, err := url.Parse(o.Host)
	if err != nil || u.Scheme != "ssh" || u.Hostname() == "" || u.User.Username() == "" {
		return nil, errors.New("ssh url must look like ssh://user@host[:port][/path/to/docker.sock]")
	}
	signer, err := ssh.ParsePrivateKey([]byte(o.SSH.PrivateKey))
	if err != nil {
		var missing *ssh.PassphraseMissingError
		if errors.As(err, &missing) {
			return nil, errors.New("ssh key is passphrase protected; use an unencrypted deploy key")
		}
		return nil, fmt.Errorf("ssh private key: %w", err)
	}

	d := &sshDialer{
		addr:   net.JoinHostPort(u.Hostname(), cmp(u.Port(), "22")),
		user:   u.User.Username(),
		socket: cmp(strings.TrimSuffix(u.Path, "/"), defaultSocket),
	}
	d.config = &ssh.ClientConfig{
		User:              d.user,
		Auth:              []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback:   d.checkHostKey(o.SSH.HostKey),
		HostKeyAlgorithms: hostKeyAlgorithms(o.SSH.HostKey),
		Timeout:           10 * time.Second,
	}

	// The host part is a placeholder; every connection goes through d.dial.
	cli, err := client.NewClientWithOpts(
		client.WithHost("http://docker.invalid"),
		client.WithDialContext(d.dial),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &Service{Client: cli, Domain: o.Domain, Log: o.Log, ssh: d, dialErr: d.dialError}, nil
}

// hostKeyAlgorithms limits negotiation to the pinned key's type. Servers have
// several host keys (RSA, ECDSA, Ed25519); without this a server could later
// present a different, equally valid key and trip the "key changed" check.
func hostKeyAlgorithms(pinned string) []string {
	if pinned == "" {
		return nil // first contact: accept the client's default preference
	}
	k, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pinned))
	if err != nil {
		return nil
	}
	if k.Type() == ssh.KeyAlgoRSA {
		return []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSA}
	}
	return []string{k.Type()}
}

func cmp(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// checkHostKey enforces a pinned key, or records the first one seen (TOFU).
func (d *sshDialer) checkHostKey(pinned string) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		seen := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
		if pinned != "" && seen != strings.TrimSpace(pinned) {
			return fmt.Errorf("ssh host key changed for %s (got %s); remove and re-add the environment if this is expected", d.addr, ssh.FingerprintSHA256(key))
		}
		d.hostKey = seen // called with d.mu held, from dial
		return nil
	}
}

func (d *sshDialer) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	c, err := d.dialLocked(ctx)
	d.lastErr = err
	return c, err
}

func (d *sshDialer) dialLocked(ctx context.Context) (net.Conn, error) {
	for attempt := 0; attempt < 2; attempt++ {
		if d.conn == nil {
			conn, err := d.connect(ctx)
			if err != nil {
				return nil, err
			}
			d.conn = conn
		}
		c, err := d.conn.Dial("unix", d.socket)
		if err == nil {
			return c, nil
		}
		// The SSH connection may have died; drop it and try once more.
		d.conn.Close()
		d.conn = nil
		if attempt == 1 {
			if strings.Contains(err.Error(), "open failed") {
				return nil, fmt.Errorf("ssh server refused to forward to %s (enable AllowStreamLocalForwarding and AllowTcpForwarding in sshd_config, and check this user can use the socket): %w", d.socket, err)
			}
			return nil, fmt.Errorf("docker socket %s over ssh: %w", d.socket, err)
		}
	}
	return nil, errors.New("unreachable")
}

// connect dials TCP with ctx (so timeouts apply) and runs the SSH handshake.
func (d *sshDialer) connect(ctx context.Context) (*ssh.Client, error) {
	tcp, err := (&net.Dialer{Timeout: d.config.Timeout}).DialContext(ctx, "tcp", d.addr)
	if err != nil {
		return nil, fmt.Errorf("ssh %s: %w", d.addr, err)
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = tcp.SetDeadline(dl)
	}
	c, chans, reqs, err := ssh.NewClientConn(tcp, d.addr, d.config)
	if err != nil {
		tcp.Close()
		return nil, fmt.Errorf("ssh %s: %w", d.addr, err)
	}
	_ = tcp.SetDeadline(time.Time{})
	return ssh.NewClient(c, chans, reqs), nil
}

func (d *sshDialer) close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn != nil {
		d.conn.Close()
		d.conn = nil
	}
}

// dialError returns the cause of the last failed SSH dial, or nil.
func (d *sshDialer) dialError() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastErr
}

// SSHHostKey returns the host key seen on the last SSH connection, or "".
func (s *Service) SSHHostKey() string {
	if s.ssh == nil {
		return ""
	}
	s.ssh.mu.Lock()
	defer s.ssh.mu.Unlock()
	return s.ssh.hostKey
}

// Close releases the Docker client and any SSH connection.
func (s *Service) Close() error {
	if s.ssh != nil {
		s.ssh.close()
	}
	return s.Client.Close()
}
