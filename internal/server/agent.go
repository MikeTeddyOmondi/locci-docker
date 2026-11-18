package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
	"github.com/MikeTeddyOmondi/locci-docker/internal/tunnel"
)

// agentConnect is where a `locci agent` dials in. The agent token (Bearer
// header) picks the environment; the WebSocket then carries that host's
// Docker connections until it drops.
//
//	GET /api/v1/agent/connect
func (s *Server) agentConnect(c *gin.Context) {
	token, _ := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if token == "" || s.Envs.Hub == nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing agent token"})
		return
	}
	e, err := s.Store.EnvironmentByAgentToken(token)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid agent token"})
		return
	}

	// Tell the agent which environment it serves.
	ws, err := s.upgrader().Upgrade(c.Writer, c.Request, http.Header{"X-Locci-Environment": {e.Name}})
	if err != nil {
		return // the upgrader already answered
	}
	who := "agent:" + e.Name
	s.Log.Info("agent connected", "env", e.Name, "remote", c.ClientIP())
	_ = s.Store.Audit(who, "CONNECT", c.Request.URL.Path, http.StatusSwitchingProtocols)
	// Check the status once the tunnel is registered, so the UI shows the
	// environment up right away instead of at the next health tick.
	_ = s.Envs.Hub.Attach(e.ID, tunnel.NewConn(ws), func() { s.checkSoon(e) }) // blocks until the tunnel closes

	s.Log.Info("agent disconnected", "env", e.Name)
	_ = s.Store.Audit(who, "DISCONNECT", c.Request.URL.Path, http.StatusOK)
	if e, err := s.Store.EnvironmentByName(e.Name); err == nil {
		s.checkSoon(e)
	}
}

// issueAgentToken creates or rotates an agent environment's token and drops
// any agent connected with the old one.
func (s *Server) issueAgentToken(e store.Environment) (string, error) {
	if e.Kind != store.KindAgent {
		return "", errors.New("only agent environments have agent tokens")
	}
	token, err := s.Store.NewAgentToken(e.ID)
	if err == nil && s.Envs.Hub != nil {
		s.Envs.Hub.Disconnect(e.ID)
	}
	return token, err
}
