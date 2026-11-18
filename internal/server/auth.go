package server

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/MikeTeddyOmondi/locci-docker/internal/store"
)

// systemUser is used for requests made with the static AUTH_TOKEN.
var systemUser = store.User{Username: "system", Role: "admin"}

func currentUser(c *gin.Context) store.User {
	u, _ := c.Get("user")
	user, _ := u.(store.User)
	return user
}

// auth accepts the static AUTH_TOKEN or a user API token, from the
// Authorization header or, for WebSockets (which can't set headers), ?token=.
func (s *Server) auth(c *gin.Context) {
	token, _ := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if token == "" && websocket.IsWebSocketUpgrade(c.Request) {
		// Only WebSockets: a token in any other URL ends up in browser
		// history and proxy access logs.
		token = c.Query("token")
	}
	if token == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
		return
	}

	if s.AuthToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.AuthToken)) == 1 {
		c.Set("user", systemUser)
		c.Next()
		return
	}

	user, err := s.Store.UserByToken(token)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}
	c.Set("user", user)
	c.Next()
}

// adminOnly must run after auth.
func adminOnly(c *gin.Context) {
	if !currentUser(c).IsAdmin() {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}
	c.Next()
}

// audit records every request that changes something.
func (s *Server) audit(c *gin.Context) {
	c.Next()
	if c.Request.Method == http.MethodGet {
		return
	}
	if err := s.Store.Audit(currentUser(c).Username, c.Request.Method, c.Request.URL.Path, c.Writer.Status()); err != nil {
		s.Log.Error("audit write failed", "err", err)
	}
}

func (s *Server) login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}

	if !s.Store.PasswordLogin() {
		s.fail(c, http.StatusForbidden, errors.New("password login is turned off; sign in with single sign-on"))
		return
	}

	// Slow down guessing: per client IP, and per username from anywhere.
	lim, ip, name := s.limits(), c.ClientIP(), strings.ToLower(req.Username)
	if d := max(lim.loginIP.wait(ip), lim.loginUser.wait(name)); d > 0 {
		_ = s.Store.Audit(req.Username, "POST", c.Request.URL.Path, http.StatusTooManyRequests)
		tooMany(c, d)
		return
	}

	user, err := s.Store.CheckPassword(req.Username, req.Password)
	if err != nil {
		lim.loginIP.add(ip)
		lim.loginUser.add(name)
		_ = s.Store.Audit(req.Username, "POST", c.Request.URL.Path, http.StatusUnauthorized)
		s.fail(c, http.StatusUnauthorized, errors.New("wrong username or password"))
		return
	}
	lim.loginUser.clear(name)

	token, err := s.Store.NewToken(user.ID, c.GetHeader("User-Agent"))
	if err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	_ = s.Store.Audit(user.Username, "POST", c.Request.URL.Path, http.StatusOK)

	c.JSON(http.StatusOK, gin.H{"token": token, "user": user})
}

func (s *Server) logout(c *gin.Context) {
	token, _ := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if err := s.Store.DeleteToken(token); err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "logged out"})
}

func (s *Server) me(c *gin.Context) {
	c.JSON(http.StatusOK, currentUser(c))
}

// ---- users (admin) ----

func (s *Server) listUsers(c *gin.Context) {
	users, err := s.Store.ListUsers()
	if err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users})
}

func (s *Server) createUser(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		Role     string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	user, err := s.Store.CreateUser(req.Username, req.Password, req.Role)
	if err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusCreated, user)
}

func (s *Server) deleteUser(c *gin.Context) {
	if err := s.Store.DeleteUser(c.Param("name")); err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			code = http.StatusNotFound
		}
		s.fail(c, code, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (s *Server) listAudit(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	entries, err := s.Store.ListAudit(limit)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"audit": entries})
}
