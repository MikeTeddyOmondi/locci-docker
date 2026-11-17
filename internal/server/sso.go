package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/MikeTeddyOmondi/locci-docker/internal/secret"
	"github.com/MikeTeddyOmondi/locci-docker/internal/sso"
	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
)

const ssoCookie = "locci_sso"

// baseURL is where browsers reach this server: LOCCI_PUBLIC_URL, or the
// request's scheme and host (honouring X-Forwarded-* from a reverse proxy).
func (s *Server) baseURL(c *gin.Context) string {
	if s.PublicURL != "" {
		return strings.TrimRight(s.PublicURL, "/")
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if p := c.GetHeader("X-Forwarded-Proto"); p != "" {
		scheme = strings.TrimSpace(strings.Split(p, ",")[0])
	}
	host := c.Request.Host
	if h := c.GetHeader("X-Forwarded-Host"); h != "" {
		host = strings.TrimSpace(strings.Split(h, ",")[0])
	}
	return scheme + "://" + host
}

func (s *Server) callbackURL(c *gin.Context, slug string) string {
	return s.baseURL(c) + "/api/v1/auth/oidc/" + url.PathEscape(slug) + "/callback"
}

// ---- public: login page and the flow ----

// authProviders tells the login page what to show. With no provider enabled
// the list is empty and only the password form appears.
//
//	GET /api/v1/auth/providers
func (s *Server) authProviders(c *gin.Context) {
	type entry struct {
		Slug        string `json:"slug"`
		DisplayName string `json:"display_name"`
	}
	out := []entry{}
	if s.SSO != nil && s.SSO.Box != nil {
		list, err := s.Store.ListProviders(true)
		if err != nil {
			s.fail(c, http.StatusInternalServerError, err)
			return
		}
		for _, p := range list {
			out = append(out, entry{p.Slug, p.DisplayName})
		}
	}
	c.JSON(http.StatusOK, gin.H{"password_login": s.Store.PasswordLogin(), "providers": out})
}

// ssoProvider returns an enabled provider, or a 404 for anything else so a
// disabled provider looks exactly like one that doesn't exist.
func (s *Server) ssoProvider(c *gin.Context) (store.Provider, bool) {
	p, err := s.Store.ProviderBySlug(c.Param("provider"))
	if err != nil || !p.Enabled || s.SSO == nil || s.SSO.Box == nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not found"})
		return p, false
	}
	return p, true
}

// ssoDone sends the browser (or the CLI's loopback listener) the outcome.
// The web token goes in the URL fragment so it never reaches server logs.
func (s *Server) ssoDone(c *gin.Context, cliPort int, token string, err error) {
	if cliPort > 0 {
		q := url.Values{}
		if err != nil {
			q.Set("error", err.Error())
		} else {
			q.Set("token", token)
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("http://127.0.0.1:%d/callback?%s", cliPort, q.Encode()))
		return
	}
	frag := url.Values{}
	if err != nil {
		frag.Set("error", err.Error())
	} else {
		frag.Set("token", token)
	}
	c.Redirect(http.StatusFound, "/login#"+frag.Encode())
}

func (s *Server) secureCookie(c *gin.Context) bool {
	return strings.HasPrefix(s.baseURL(c), "https://")
}

func (s *Server) setFlowCookie(c *gin.Context, value string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: ssoCookie, Value: value, Path: "/api/v1/auth/oidc/", MaxAge: maxAge,
		HttpOnly: true, Secure: s.secureCookie(c),
		// Lax lets the cookie come back on the provider's top-level redirect.
		SameSite: http.SameSiteLaxMode,
	})
}

// ssoStart sends the user to the provider.
//
//	GET /api/v1/auth/oidc/:provider/start[?cli_port=N]
func (s *Server) ssoStart(c *gin.Context) {
	p, ok := s.ssoProvider(c)
	if !ok {
		return
	}
	// The CLI's listener is always on 127.0.0.1; only a port is accepted.
	cliPort, _ := strconv.Atoi(c.Query("cli_port"))
	if cliPort < 1024 || cliPort > 65535 {
		cliPort = 0
	}
	authURL, cookie, err := s.SSO.Start(c.Request.Context(), p, s.callbackURL(c, p.Slug), cliPort)
	if err != nil {
		s.Log.Warn("sso start", "provider", p.Slug, "err", err)
		s.ssoDone(c, cliPort, "", fmt.Errorf("couldn't reach %s: %w", p.DisplayName, err))
		return
	}
	s.setFlowCookie(c, cookie, int(sso.FlowTTL.Seconds()))
	c.Redirect(http.StatusFound, authURL)
}

// ssoCallback finishes the flow and signs the user in.
//
//	GET /api/v1/auth/oidc/:provider/callback?code=...&state=...
func (s *Server) ssoCallback(c *gin.Context) {
	p, ok := s.ssoProvider(c)
	if !ok {
		return
	}
	s.setFlowCookie(c, "", -1) // single use

	// Not c.Cookie: gin query-unescapes the value, turning base64 "+" into spaces.
	raw := ""
	if ck, err := c.Request.Cookie(ssoCookie); err == nil {
		raw = ck.Value
	}
	flow, err := s.SSO.OpenFlow(raw)
	who := "sso:" + p.Slug
	fail := func(err error) {
		_ = s.Store.Audit(who, "LOGIN", c.Request.URL.Path, http.StatusUnauthorized)
		s.ssoDone(c, flow.CLIPort, "", err)
	}
	if err != nil {
		fail(err)
		return
	}
	if e := c.Query("error"); e != "" {
		if d := c.Query("error_description"); d != "" {
			e = d
		}
		fail(fmt.Errorf("%s: %s", p.DisplayName, e))
		return
	}

	claims, err := s.SSO.Finish(c.Request.Context(), p, s.callbackURL(c, p.Slug), flow, c.Query("state"), c.Query("code"))
	if err != nil {
		s.Log.Warn("sso finish", "provider", p.Slug, "err", err)
		fail(err)
		return
	}
	if claims.Email != "" {
		who = "sso:" + p.Slug + ":" + claims.Email
	}
	user, err := s.SSO.Resolve(p, claims)
	if err != nil {
		fail(err)
		return
	}
	token, err := s.Store.NewToken(user.ID, "sso:"+p.Slug)
	if err != nil {
		fail(err)
		return
	}
	_ = s.Store.Audit(user.Username, "LOGIN", c.Request.URL.Path, http.StatusOK)
	s.ssoDone(c, flow.CLIPort, token, nil)
}

// ---- admin: providers and settings ----

// providerView adds what the admin needs and hides the client secret.
type providerView struct {
	store.Provider
	HasSecret   bool   `json:"has_secret"`
	RedirectURI string `json:"redirect_uri"` // register this at the provider
}

func (s *Server) providerView(c *gin.Context, p store.Provider) providerView {
	return providerView{Provider: p, HasSecret: p.HasSecret(), RedirectURI: s.callbackURL(c, p.Slug)}
}

// providerRequest: nil fields are left alone on update; ClientSecret "" keeps
// the stored secret unless ClearSecret is set (public clients have none).
type providerRequest struct {
	Slug            string  `json:"slug"`
	DisplayName     *string `json:"display_name"`
	IssuerURL       *string `json:"issuer_url"`
	ClientID        *string `json:"client_id"`
	ClientSecret    string  `json:"client_secret"`
	ClearSecret     bool    `json:"clear_secret"`
	Scopes          *string `json:"scopes"`
	Enabled         *bool   `json:"enabled"`
	AutoCreateUsers *bool   `json:"auto_create_users"`
	DefaultRole     *string `json:"default_role"`
	AdminGroup      *string `json:"admin_group"`
	GroupsClaim     *string `json:"groups_claim"`
	AllowedDomains  *string `json:"allowed_email_domains"`
}

func (r providerRequest) apply(p *store.Provider) {
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = strings.TrimSpace(*v)
		}
	}
	set(&p.DisplayName, r.DisplayName)
	set(&p.IssuerURL, r.IssuerURL)
	set(&p.ClientID, r.ClientID)
	set(&p.Scopes, r.Scopes)
	set(&p.DefaultRole, r.DefaultRole)
	set(&p.AdminGroup, r.AdminGroup)
	set(&p.GroupsClaim, r.GroupsClaim)
	set(&p.AllowedDomains, r.AllowedDomains)
	if r.Enabled != nil {
		p.Enabled = *r.Enabled
	}
	if r.AutoCreateUsers != nil {
		p.AutoCreateUsers = *r.AutoCreateUsers
	}
}

// ssoBox returns the box for client secrets, or answers 412 when
// LOCCI_SECRET_KEY isn't set (sign-in cookies need it too).
func (s *Server) ssoBox(c *gin.Context) (*secret.Box, bool) {
	if s.SSO == nil || s.SSO.Box == nil {
		s.fail(c, http.StatusPreconditionFailed, fmt.Errorf("single sign-on needs a secret key: %w", secret.ErrNoKey))
		return nil, false
	}
	return s.SSO.Box, true
}

func (s *Server) listProviders(c *gin.Context) {
	list, err := s.Store.ListProviders(false)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	out := make([]providerView, len(list))
	for i, p := range list {
		out[i] = s.providerView(c, p)
	}
	c.JSON(http.StatusOK, gin.H{"providers": out, "password_login": s.Store.PasswordLogin()})
}

func (s *Server) createProvider(c *gin.Context) {
	var req providerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	box, ok := s.ssoBox(c)
	if !ok {
		return
	}
	p := store.Provider{Slug: strings.TrimSpace(req.Slug)}
	req.apply(&p)
	var err error
	if p.ClientSecret, err = box.Seal(req.ClientSecret); err != nil {
		s.sealFailure(c, err)
		return
	}
	saved, err := s.Store.CreateProvider(p)
	if err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusCreated, s.providerView(c, saved))
}

func (s *Server) updateProvider(c *gin.Context) {
	var req providerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	box, ok := s.ssoBox(c)
	if !ok {
		return
	}
	p, err := s.Store.ProviderBySlug(c.Param("provider"))
	if err != nil {
		s.fail(c, notFoundOr500(err), err)
		return
	}
	oldIssuer := p.IssuerURL
	req.apply(&p)
	switch {
	case req.ClearSecret:
		p.ClientSecret = ""
	case req.ClientSecret != "":
		if p.ClientSecret, err = box.Seal(req.ClientSecret); err != nil {
			s.sealFailure(c, err)
			return
		}
	}
	// Keep at least one way in: the last enabled provider can't be disabled
	// while password login is off.
	if !p.Enabled && !s.Store.PasswordLogin() {
		if on, _ := s.Store.ListProviders(true); len(on) <= 1 {
			s.fail(c, http.StatusConflict, errors.New("password login is off; turn it on before disabling the last provider"))
			return
		}
	}
	saved, err := s.Store.UpdateProvider(p)
	if err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	s.SSO.Forget(oldIssuer)
	c.JSON(http.StatusOK, s.providerView(c, saved))
}

func (s *Server) deleteProvider(c *gin.Context) {
	p, err := s.Store.ProviderBySlug(c.Param("provider"))
	if err != nil {
		s.fail(c, notFoundOr500(err), err)
		return
	}
	if p.Enabled && !s.Store.PasswordLogin() {
		if on, _ := s.Store.ListProviders(true); len(on) <= 1 {
			s.fail(c, http.StatusConflict, errors.New("password login is off; turn it on before removing the last provider"))
			return
		}
	}
	if err := s.Store.DeleteProvider(p.Slug); err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	if s.SSO != nil {
		s.SSO.Forget(p.IssuerURL)
	}
	c.JSON(http.StatusOK, gin.H{"deleted": p.Slug})
}

// testProvider runs discovery for an issuer and reports what it found.
//
//	POST /api/v1/sso/providers/test {"issuer_url": "..."}
func (s *Server) testProvider(c *gin.Context) {
	var req struct {
		IssuerURL string `json:"issuer_url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	if s.SSO == nil {
		s.fail(c, http.StatusPreconditionFailed, secret.ErrNoKey)
		return
	}
	s.SSO.Forget(req.IssuerURL)
	op, err := s.SSO.Discover(c.Request.Context(), req.IssuerURL)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	var meta struct {
		Scopes []string `json:"scopes_supported"`
		PKCE   []string `json:"code_challenge_methods_supported"`
	}
	_ = op.Claims(&meta)
	warnings := []string{}
	if len(meta.PKCE) > 0 && !slices.Contains(meta.PKCE, "S256") {
		warnings = append(warnings, "the provider doesn't list S256 PKCE; sign-in may fail")
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "issuer": req.IssuerURL,
		"authorization_endpoint": op.Endpoint().AuthURL, "token_endpoint": op.Endpoint().TokenURL,
		"userinfo_endpoint": op.UserInfoEndpoint(), "scopes_supported": meta.Scopes, "warnings": warnings})
}

// ssoSettings reads or changes the password-login switch.
//
//	GET/PUT /api/v1/sso/settings {"password_login": false}
func (s *Server) getSSOSettings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"password_login": s.Store.PasswordLogin(), "public_url": s.baseURL(c)})
}

func (s *Server) putSSOSettings(c *gin.Context) {
	var req struct {
		PasswordLogin *bool `json:"password_login" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	if !*req.PasswordLogin {
		if on, _ := s.Store.ListProviders(true); len(on) == 0 || s.SSO == nil || s.SSO.Box == nil {
			s.fail(c, http.StatusConflict, errors.New("enable a sign-in provider before turning password login off"))
			return
		}
	}
	if err := s.Store.SetPasswordLogin(*req.PasswordLogin); err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	s.getSSOSettings(c)
}
