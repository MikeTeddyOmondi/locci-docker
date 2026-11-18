package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// The web UI's sign-in lives in an HttpOnly cookie, out of reach of page
// scripts. Requests that change something must also carry csrfHeader: a page
// on another site can't add a custom header without a CORS preflight, which
// locci never allows. SameSite=Lax keeps the cookie off cross-site POSTs as
// well, while still sending it after the identity provider's cross-site
// redirect back (Strict can drop it there). The CLI and other API clients
// keep using Authorization: Bearer, which needs neither.
const (
	sessionCookie = "locci_session"
	csrfHeader    = "X-Locci-CSRF"
)

// setSession stores token in the session cookie. The server decides when the
// token expires (LOCCI_TOKEN_TTL_DAYS); the cookie lasts as long as browsers
// allow.
func (s *Server) setSession(c *gin.Context, token string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", MaxAge: 400 * 24 * 3600,
		HttpOnly: true, Secure: s.secureCookie(c), SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSession(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.secureCookie(c), SameSite: http.SameSiteLaxMode,
	})
}

// sessionToken returns the session cookie's token, or "".
func sessionToken(c *gin.Context) string {
	if ck, err := c.Request.Cookie(sessionCookie); err == nil {
		return ck.Value
	}
	return ""
}

// safeMethod reports whether a request only reads.
func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// adoptSession moves a bearer token into the session cookie: the web UI calls
// it once for a token it kept in local storage before the cookie existed.
//
//	POST /api/v1/auth/session   (Authorization: Bearer <token>)
func (s *Server) adoptSession(c *gin.Context) {
	if c.GetBool("viaCookie") {
		c.JSON(http.StatusOK, gin.H{"status": "already a session"})
		return
	}
	if currentUser(c).ID == 0 {
		s.fail(c, http.StatusBadRequest, errors.New("the static AUTH_TOKEN can't become a browser session; sign in as a user"))
		return
	}
	s.setSession(c, c.GetString("token"))
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
