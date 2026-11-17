package docker

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// fakeDocker answers /_ping like a Docker daemon would.
var fakeDocker = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/_ping") {
		w.Header().Set("API-Version", "1.47")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
		return
	}
	http.NotFound(w, r)
})

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return c
}

// ---- TLS ----

func pemBlock(typ string, der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}))
}

func newCert(t *testing.T, tmpl, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey, string, string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if parent == nil {
		parent, parentKey = tmpl, key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	kder, _ := x509.MarshalECPrivateKey(key)
	return cert, key, pemBlock("CERTIFICATE", der), pemBlock("EC PRIVATE KEY", kder)
}

func TestTLSHost(t *testing.T) {
	now := time.Now()
	ca, caKey, caPEM, _ := newCert(t, &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ca"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}, nil, nil)
	_, srvKey, srvPEM, _ := newCert(t, &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "server"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, ca, caKey)
	_, _, cliPEM, cliKeyPEM := newCert(t, &x509.Certificate{
		SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "client"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}, ca, caKey)

	srvKeyDER, _ := x509.MarshalECPrivateKey(srvKey)
	pair, err := tls.X509KeyPair([]byte(srvPEM), []byte(pemBlock("EC PRIVATE KEY", srvKeyDER)))
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)

	srv := httptest.NewUnstartedServer(fakeDocker)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	host := "tcp://" + srv.Listener.Addr().String()

	ok, err := NewWithOptions(Options{Host: host, Log: quiet, TLS: &TLSOptions{CA: caPEM, Cert: cliPEM, Key: cliKeyPEM}})
	if err != nil {
		t.Fatal(err)
	}
	defer ok.Close()
	if err := ok.Ping(ctx(t)); err != nil {
		t.Fatalf("ping over tls: %v", err)
	}

	// Incomplete TLS material is rejected before any connection is made.
	if _, err := NewWithOptions(Options{Host: host, Log: quiet, TLS: &TLSOptions{CA: caPEM}}); err == nil {
		t.Fatal("expected missing cert/key to be rejected")
	}
}

// ---- SSH ----

// sshServer forwards direct-streamlocal channels to the given unix socket,
// like sshd does for `ssh -L` to a socket. It returns its address and host key.
func sshServer(t *testing.T, authorized ssh.PublicKey, socket string) (string, ssh.PublicKey) {
	t.Helper()
	_, hostPriv, _ := ed25519.GenerateKey(rand.Reader)
	hostSigner, _ := ssh.NewSignerFromKey(hostPriv)

	cfg := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
		if string(k.Marshal()) == string(authorized.Marshal()) {
			return nil, nil
		}
		return nil, io.EOF
	}}
	cfg.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			tcp, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_, chans, reqs, err := ssh.NewServerConn(tcp, cfg)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for nc := range chans {
					if nc.ChannelType() != "direct-streamlocal@openssh.com" {
						nc.Reject(ssh.UnknownChannelType, "nope")
						continue
					}
					// payload: string socket path, string reserved, uint32 reserved
					p := nc.ExtraData()
					n := binary.BigEndian.Uint32(p[:4])
					path := string(p[4 : 4+n])
					up, err := net.Dial("unix", path)
					if err != nil {
						nc.Reject(ssh.ConnectionFailed, err.Error())
						continue
					}
					ch, creqs, _ := nc.Accept()
					go ssh.DiscardRequests(creqs)
					go func() { io.Copy(ch, up); ch.CloseWrite() }()
					go func() { io.Copy(up, ch); up.Close() }()
				}
			}()
		}
	}()
	return ln.Addr().String(), hostSigner.PublicKey()
}

func TestSSHHost(t *testing.T) {
	// Fake Docker daemon on a unix socket (short path: macOS limits socket paths).
	dir, err := os.MkdirTemp("", "dk")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "d.sock")
	ul, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	hs := &http.Server{Handler: fakeDocker}
	go hs.Serve(ul)
	t.Cleanup(func() { hs.Close() })

	_, userPriv, _ := ed25519.GenerateKey(rand.Reader)
	userSigner, _ := ssh.NewSignerFromKey(userPriv)
	block, _ := ssh.MarshalPrivateKey(userPriv, "")
	keyPEM := string(pem.EncodeToMemory(block))

	addr, hostKey := sshServer(t, userSigner.PublicKey(), sock)
	host := "ssh://deploy@" + addr + sock

	// First connect: no pinned key, so the server's key is recorded (TOFU).
	s, err := NewWithOptions(Options{Host: host, Log: quiet, SSH: &SSHOptions{PrivateKey: keyPEM}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Ping(ctx(t)); err != nil {
		t.Fatalf("ping over ssh: %v", err)
	}
	seen := s.SSHHostKey()
	want := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(hostKey)))
	if seen != want {
		t.Fatalf("host key %q, want %q", seen, want)
	}

	// A dropped SSH connection is redialled on the next request.
	s.ssh.close()
	if err := s.Ping(ctx(t)); err != nil {
		t.Fatalf("ping after reconnect: %v", err)
	}
	s.Close()

	// Pinned key matches: fine.
	pinned, _ := NewWithOptions(Options{Host: host, Log: quiet, SSH: &SSHOptions{PrivateKey: keyPEM, HostKey: seen}})
	if err := pinned.Ping(ctx(t)); err != nil {
		t.Fatalf("ping with pinned key: %v", err)
	}
	pinned.Close()

	// Pinned key differs: refused.
	_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	otherSigner, _ := ssh.NewSignerFromKey(otherPriv)
	wrong := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(otherSigner.PublicKey())))
	bad, _ := NewWithOptions(Options{Host: host, Log: quiet, SSH: &SSHOptions{PrivateKey: keyPEM, HostKey: wrong}})
	if err := bad.Ping(ctx(t)); err == nil || !strings.Contains(err.Error(), "host key changed") {
		t.Fatalf("expected host key mismatch, got %v", err)
	}
	bad.Close()

	// Bad configuration fails up front.
	if _, err := NewWithOptions(Options{Host: "ssh://nouser-host", SSH: &SSHOptions{PrivateKey: keyPEM}}); err == nil {
		t.Fatal("expected url without user to be rejected")
	}
	if _, err := NewWithOptions(Options{Host: host, SSH: &SSHOptions{PrivateKey: "junk"}}); err == nil {
		t.Fatal("expected a bad key to be rejected")
	}
}

func TestHostKeyAlgorithms(t *testing.T) {
	if hostKeyAlgorithms("") != nil || hostKeyAlgorithms("junk") != nil {
		t.Fatal("no pin should leave negotiation alone")
	}
	ek, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pub, _ := ssh.NewPublicKey(&ek.PublicKey)
	got := hostKeyAlgorithms(string(ssh.MarshalAuthorizedKey(pub)))
	if len(got) != 1 || got[0] != "ecdsa-sha2-nistp256" {
		t.Fatalf("ecdsa pin: %v", got)
	}
}
