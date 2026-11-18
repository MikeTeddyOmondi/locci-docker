package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/MikeTeddyOmondi/locci-docker/internal/docker"
	"github.com/MikeTeddyOmondi/locci-docker/internal/env"
	"github.com/MikeTeddyOmondi/locci-docker/internal/secret"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
)

// envView is what the API returns: never the secret, only whether one is set.
type envView struct {
	store.Environment
	HasSecret      bool   `json:"has_secret"`
	Fingerprint    string `json:"ssh_fingerprint,omitempty"`
	AgentConnected *bool  `json:"agent_connected,omitempty"` // agent environments only, live
	AgentToken     string `json:"agent_token,omitempty"`     // only right after it is issued
}

func (s *Server) view(e store.Environment) envView {
	v := envView{Environment: e, HasSecret: e.HasSecret(), Fingerprint: env.Fingerprint(e.SSHHostKey)}
	if e.Kind == store.KindAgent {
		on := s.Envs.Hub != nil && s.Envs.Hub.Connected(e.ID)
		v.AgentConnected = &on
	}
	return v
}

// envRequest is the body for create, update and test. TLSKey and SSHKey are
// write-only. On update, empty strings and a nil Labels mean "keep".
type envRequest struct {
	Name         string            `json:"name"`
	Label        string            `json:"label"`
	Kind         string            `json:"kind"`
	URL          string            `json:"url"`
	TLSCA        string            `json:"tls_ca"`
	TLSCert      string            `json:"tls_cert"`
	TLSKey       string            `json:"tls_key"`
	SSHKey       string            `json:"ssh_key"`
	SSHHostKey   string            `json:"ssh_host_key"`       // from a previous test, to pin
	ResetHostKey bool              `json:"reset_host_key"`     // update only: forget the pinned key
	RotateToken  bool              `json:"rotate_agent_token"` // update only: issue a new agent token
	Labels       map[string]string `json:"labels"`
}

func (r envRequest) environment() store.Environment {
	return store.Environment{
		Name: r.Name, Label: r.Label, Kind: r.Kind, URL: r.URL,
		TLSCA: r.TLSCA, TLSCert: r.TLSCert, SSHHostKey: r.SSHHostKey, Labels: r.Labels,
	}
}

// key returns the plaintext secret for the request's kind.
func (r envRequest) key(kind string) string {
	switch kind {
	case store.KindTLS:
		return r.TLSKey
	case store.KindSSH:
		return r.SSHKey
	}
	return ""
}

// checkOptions validates connection settings without connecting: bad PEM, a
// passphrase-protected key or a malformed URL are reported as 400s.
func checkOptions(o docker.Options) error {
	svc, err := docker.NewWithOptions(o)
	if err != nil {
		return err
	}
	return svc.Close()
}

// sealFailure maps secret errors: a missing key is a server setup problem the
// admin must fix, so say so plainly.
func (s *Server) sealFailure(c *gin.Context, err error) {
	if errors.Is(err, secret.ErrNoKey) {
		s.fail(c, http.StatusPreconditionFailed, err)
		return
	}
	s.fail(c, http.StatusInternalServerError, err)
}

func (s *Server) listEnvironments(c *gin.Context) {
	list, err := s.Store.ListEnvironments()
	if err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	out := make([]envView, len(list))
	for i, e := range list {
		out[i] = s.view(e)
	}
	c.JSON(http.StatusOK, gin.H{"environments": out})
}

func (s *Server) getEnvironment(c *gin.Context) {
	e, err := s.Store.EnvironmentByName(c.Param("env"))
	if err != nil {
		s.fail(c, notFoundOr500(err), err)
		return
	}
	c.JSON(http.StatusOK, s.view(e))
}

func notFoundOr500(err error) int {
	if errors.Is(err, store.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

// testEnvironment connects to unsaved settings once and reports the host.
//
//	POST /api/v1/environments/test
//
// When editing, the body names the existing environment and may leave the
// certificates and key blank: those are taken from the stored environment
// (server side only, nothing is returned). With the same URL the pinned SSH
// host key is enforced too, so a test also catches a changed host key.
func (s *Server) testEnvironment(c *gin.Context) {
	var req envRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	if req.Kind == store.KindAgent {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "an agent environment connects to the server itself: save it, run `locci agent` on the host, and watch its status"})
		return
	}
	e, key := req.environment(), req.key(req.Kind)
	if stored, err := s.Store.EnvironmentByName(req.Name); err == nil && stored.Kind == req.Kind {
		if e.URL == "" {
			e.URL = stored.URL
		}
		if e.TLSCA == "" {
			e.TLSCA = stored.TLSCA
		}
		if e.TLSCert == "" {
			e.TLSCert = stored.TLSCert
		}
		if e.SSHHostKey == "" && e.URL == stored.URL && !req.ResetHostKey {
			e.SSHHostKey = stored.SSHHostKey
		}
		if key == "" {
			if key, err = s.Envs.Box.Open(stored.Secret); err != nil {
				s.sealFailure(c, err)
				return
			}
		}
	}
	o, err := s.Envs.PlainOptions(e, key)
	if err == nil {
		err = checkOptions(o)
	}
	if err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	res, err := env.Test(c.Request.Context(), o)
	if err != nil {
		// Reachability is the answer here, not a server failure.
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "host": res})
}

func (s *Server) createEnvironment(c *gin.Context) {
	var req envRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	e := req.environment()
	key := req.key(e.Kind)

	o, err := s.Envs.PlainOptions(e, key)
	if err == nil && e.Kind != store.KindLocal {
		err = checkOptions(o)
	}
	if err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}

	// SSH: pin the host key now if the caller didn't bring one from a test and
	// the host is reachable. Otherwise the health loop pins it on first contact.
	if e.Kind == store.KindSSH && e.SSHHostKey == "" {
		if res, terr := env.Test(c.Request.Context(), o); terr == nil {
			e.SSHHostKey = res.SSHHostKey
		}
	}

	if e.Secret, err = s.Envs.Box.Seal(key); err != nil {
		s.sealFailure(c, err)
		return
	}
	saved, err := s.Store.CreateEnvironment(e)
	if err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	v := s.view(saved)
	if saved.Kind == store.KindAgent {
		// Shown once: only its hash is kept.
		if v.AgentToken, err = s.issueAgentToken(saved); err != nil {
			s.fail(c, http.StatusInternalServerError, err)
			return
		}
	}
	s.checkSoon(saved)
	c.JSON(http.StatusCreated, v)
}

func (s *Server) updateEnvironment(c *gin.Context) {
	var req envRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	e, err := s.Store.EnvironmentByName(c.Param("env"))
	if err != nil {
		s.fail(c, notFoundOr500(err), err)
		return
	}
	if req.Kind != "" && req.Kind != e.Kind {
		s.fail(c, http.StatusBadRequest, errors.New("kind can't change; add a new environment instead"))
		return
	}

	if req.Label != "" {
		e.Label = req.Label
	}
	if req.URL != "" && req.URL != e.URL {
		e.URL = req.URL
		e.SSHHostKey = "" // a different host has a different key
	}
	if req.TLSCA != "" {
		e.TLSCA = req.TLSCA
	}
	if req.TLSCert != "" {
		e.TLSCert = req.TLSCert
	}
	if req.Labels != nil {
		e.Labels = req.Labels
	}
	if req.ResetHostKey {
		e.SSHHostKey = ""
	}
	if req.SSHHostKey != "" {
		e.SSHHostKey = req.SSHHostKey
	}

	// Validate the merged settings with the new key, or the stored one.
	key := req.key(e.Kind)
	if key == "" {
		if key, err = s.Envs.Box.Open(e.Secret); err != nil {
			s.sealFailure(c, err)
			return
		}
	} else if e.Secret, err = s.Envs.Box.Seal(key); err != nil {
		s.sealFailure(c, err)
		return
	}
	if e.Kind != store.KindLocal && e.Kind != store.KindAgent {
		o, err := s.Envs.PlainOptions(e, key)
		if err == nil {
			err = checkOptions(o)
		}
		if err != nil {
			s.fail(c, http.StatusBadRequest, err)
			return
		}
	}

	saved, err := s.Store.UpdateEnvironment(e)
	if err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	v := s.view(saved)
	if req.RotateToken {
		if v.AgentToken, err = s.issueAgentToken(saved); err != nil {
			s.fail(c, http.StatusBadRequest, err)
			return
		}
		v = s.withToken(saved, v.AgentToken)
	}
	s.checkSoon(saved)
	c.JSON(http.StatusOK, v)
}

func (s *Server) deleteEnvironment(c *gin.Context) {
	e, err := s.Store.EnvironmentByName(c.Param("env"))
	if err != nil {
		s.fail(c, notFoundOr500(err), err)
		return
	}
	if err := s.Store.DeleteEnvironment(e.Name); err != nil {
		s.fail(c, http.StatusConflict, err) // the default can't be removed
		return
	}
	s.Envs.Forget(e.ID)
	if s.Envs.Hub != nil {
		s.Envs.Hub.Disconnect(e.ID)
	}
	c.JSON(http.StatusOK, gin.H{"deleted": e.Name})
}

func (s *Server) setDefaultEnvironment(c *gin.Context) {
	if err := s.Store.SetDefaultEnvironment(c.Param("env")); err != nil {
		s.fail(c, notFoundOr500(err), err)
		return
	}
	e, _ := s.Store.EnvironmentByName(c.Param("env"))
	c.JSON(http.StatusOK, s.view(e))
}

// withToken is the view of e with a freshly issued agent token attached (the
// rotation dropped the agent, so connected is recomputed).
func (s *Server) withToken(e store.Environment, token string) envView {
	v := s.view(e)
	v.AgentToken = token
	return v
}

// checkSoon refreshes an environment's status in the background so the UI
// shows up/down without waiting for the next health loop tick.
func (s *Server) checkSoon(e store.Environment) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), env.HealthTimeout+2*time.Second)
		defer cancel()
		_ = s.Envs.Check(ctx, e)
	}()
}
