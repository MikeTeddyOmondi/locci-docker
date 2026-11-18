package secret

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func newKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func TestRoundTrip(t *testing.T) {
	box, err := New(newKey(t))
	if err != nil {
		t.Fatal(err)
	}
	const plain = "-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n"

	a, err := box.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := box.Seal(plain)
	if a == b {
		t.Fatal("expected a fresh nonce per seal")
	}
	if !strings.HasPrefix(a, "v1:") || strings.Contains(a, "PRIVATE") {
		t.Fatalf("sealed value looks wrong: %q", a)
	}
	got, err := box.Open(a)
	if err != nil || got != plain {
		t.Fatalf("open: %v %q", err, got)
	}

	if s, err := box.Seal(""); err != nil || s != "" {
		t.Fatalf("empty seal: %v %q", err, s)
	}
}

func TestTamperAndWrongKey(t *testing.T) {
	box, _ := New(newKey(t))
	sealed, _ := box.Seal("secret")

	// Flip a byte in the ciphertext.
	raw, _ := base64.StdEncoding.DecodeString(sealed[3:])
	raw[len(raw)-1] ^= 1
	if _, err := box.Open("v1:" + base64.StdEncoding.EncodeToString(raw)); err == nil {
		t.Fatal("expected tampered value to fail")
	}

	other, _ := New(newKey(t))
	if _, err := other.Open(sealed); err == nil {
		t.Fatal("expected wrong key to fail")
	}
	if _, err := box.Open("plain-text"); err == nil {
		t.Fatal("expected unknown format to fail")
	}
}

func TestNoKey(t *testing.T) {
	box, err := New("  ")
	if err != nil || box != nil {
		t.Fatalf("empty key should give a nil box: %v", err)
	}
	if _, err := box.Seal("x"); !errors.Is(err, ErrNoKey) {
		t.Fatalf("expected ErrNoKey, got %v", err)
	}
	if _, err := box.Open("v1:abc"); !errors.Is(err, ErrNoKey) {
		t.Fatalf("expected ErrNoKey, got %v", err)
	}
	if _, err := New("too-short"); err == nil {
		t.Fatal("expected a bad key to be rejected")
	}
}
