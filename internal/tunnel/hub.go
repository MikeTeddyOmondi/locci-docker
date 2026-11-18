package tunnel

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
)

// ErrNotConnected means the environment's agent isn't connected right now.
var ErrNotConnected = errors.New("the agent for this environment is not connected")

func config() *yamux.Config {
	c := yamux.DefaultConfig()
	c.LogOutput = io.Discard
	c.KeepAliveInterval = 20 * time.Second // also keeps proxies from idling the WebSocket out
	return c
}

// Hub holds the connected agents on the server, one session per environment.
type Hub struct {
	mu       sync.Mutex
	sessions map[int64]*yamux.Session
}

// Attach makes conn the tunnel for an environment, replacing (and closing)
// any previous one, and returns once the tunnel closes. ready, if not nil,
// runs as soon as the environment can be dialled.
func (h *Hub) Attach(envID int64, conn net.Conn, ready func()) error {
	// The server opens streams, so it is yamux's client side.
	sess, err := yamux.Client(conn, config())
	if err != nil {
		return err
	}
	h.mu.Lock()
	if h.sessions == nil {
		h.sessions = map[int64]*yamux.Session{}
	}
	if old := h.sessions[envID]; old != nil {
		old.Close()
	}
	h.sessions[envID] = sess
	h.mu.Unlock()
	if ready != nil {
		ready() // the environment can be dialled from here on
	}

	<-sess.CloseChan()

	h.mu.Lock()
	if h.sessions[envID] == sess {
		delete(h.sessions, envID)
	}
	h.mu.Unlock()
	return nil
}

// Dial opens a new connection to the environment's Docker socket.
func (h *Hub) Dial(_ context.Context, envID int64) (net.Conn, error) {
	h.mu.Lock()
	sess := h.sessions[envID]
	h.mu.Unlock()
	if sess == nil || sess.IsClosed() {
		return nil, ErrNotConnected
	}
	return sess.Open()
}

// Connected reports whether an agent is attached for the environment.
func (h *Hub) Connected(envID int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	sess := h.sessions[envID]
	return sess != nil && !sess.IsClosed()
}

// Disconnect closes an environment's tunnel (after its token was rotated or
// the environment removed).
func (h *Hub) Disconnect(envID int64) {
	h.mu.Lock()
	sess := h.sessions[envID]
	h.mu.Unlock()
	if sess != nil {
		sess.Close()
	}
}
