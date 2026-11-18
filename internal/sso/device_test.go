package sso

import (
	"context"
	"errors"
	"testing"

	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
)

func TestDeviceFlow(t *testing.T) {
	f := setup(t, nil)
	ctx := context.Background()
	f.idp.SetUser(map[string]any{"sub": "d-1", "email": "dee@example.com", "email_verified": true, "groups": []string{"locci-admins"}})

	dc, err := f.svc.DeviceStart(ctx, f.p)
	if err != nil {
		t.Fatal(err)
	}
	if dc.UserCode != "ABCD-EFGH" || dc.VerificationURI == "" || dc.Interval != 1 || dc.Handle == "" || dc.ExpiresIn <= 0 {
		t.Fatalf("device code: %+v", dc)
	}

	// Not approved yet.
	if _, err := f.svc.DevicePoll(ctx, f.p, dc.Handle); !errors.Is(err, ErrPending) {
		t.Fatalf("expected pending, got %v", err)
	}

	// Approved: verified claims (no nonce in this flow), then the usual user rules.
	f.idp.ApproveDevice()
	c, err := f.svc.DevicePoll(ctx, f.p, dc.Handle)
	if err != nil || c.Subject != "d-1" || c.Email != "dee@example.com" {
		t.Fatalf("approved: %v %+v", err, c)
	}
	if u, err := f.svc.Resolve(f.p, c); err != nil || u.Role != "admin" {
		t.Fatalf("resolve: %v %+v", err, u)
	}

	// A handle can't be forged or moved to another provider.
	if _, err := f.svc.DevicePoll(ctx, f.p, dc.Handle+"x"); err == nil {
		t.Fatal("expected a tampered handle to fail")
	}
	other := f.p
	other.Slug = "other"
	if _, err := f.svc.DevicePoll(ctx, other, dc.Handle); err == nil {
		t.Fatal("expected a handle from another provider to fail")
	}

	// Denied by the user.
	dc2, _ := f.svc.DeviceStart(ctx, f.p)
	f.idp.DenyDevice()
	if _, err := f.svc.DevicePoll(ctx, f.p, dc2.Handle); err == nil || errors.Is(err, ErrPending) {
		t.Fatalf("expected a denial, got %v", err)
	}
}

func TestDeviceFlowUnsupported(t *testing.T) {
	f := setup(t, nil)
	f.idp.NoDevice = true
	f.svc.Forget(f.p.IssuerURL)
	if _, err := f.svc.DeviceStart(context.Background(), f.p); !errors.Is(err, ErrNoDeviceFlow) {
		t.Fatalf("expected ErrNoDeviceFlow, got %v", err)
	}
}

func TestDeviceFlowWrongSecret(t *testing.T) {
	f := setup(t, func(p *store.Provider) {})
	bad, _ := f.svc.Box.Seal("wrong")
	f.p.ClientSecret = bad
	if _, err := f.svc.DeviceStart(context.Background(), f.p); err == nil {
		t.Fatal("expected the device request to fail without the right client secret")
	}
}
