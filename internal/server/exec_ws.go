package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// wsPing keeps idle WebSockets alive through proxies that close quiet
// connections (NGINX after 60 seconds by default, Cloudflare after 100).
const wsPing = 30 * time.Second

// upgrader accepts WebSockets from the UI's own origin only. Behind a reverse
// proxy the Host header may be the upstream's (NGINX's proxy_pass without
// proxy_set_header Host), so the public host also counts: from
// LOCCI_PUBLIC_URL, or X-Forwarded-Host from a trusted proxy. Requests
// without Origin (the CLI, agents) aren't from a browser page.
func (s *Server) upgrader() *websocket.Upgrader {
	return &websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     s.sameOrigin,
	}
}

func (s *Server) sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Host)
	allowed := []string{r.Host}
	if s.PublicURL != "" {
		if p, err := url.Parse(s.PublicURL); err == nil {
			allowed = append(allowed, p.Host)
		}
	}
	if s.trustedPeer(r) {
		if h := r.Header.Get("X-Forwarded-Host"); h != "" {
			allowed = append(allowed, strings.TrimSpace(strings.Split(h, ",")[0]))
		}
	}
	for _, a := range allowed {
		if a != "" && strings.EqualFold(host, a) {
			return true
		}
	}
	return false
}

// keepAlive pings ws until done closes. WriteControl may run alongside the
// other writer.
func keepAlive(ws *websocket.Conn, done <-chan struct{}) {
	t := time.NewTicker(wsPing)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			if ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)) != nil {
				return
			}
		}
	}
}

// execWS bridges a browser terminal to an interactive exec session.
//
//	GET /api/v1/containers/:id/exec/ws?cmd=sh&token=...
//
// Binary/text frames from the client are stdin, except JSON text frames
// shaped like {"resize":[cols,rows]}. Output is sent as binary frames.
func (s *Server) execWS(c *gin.Context) {
	cmd := strings.Fields(c.DefaultQuery("cmd", "sh"))

	// Upgrade first so the origin check runs before anything executes.
	ws, err := s.upgrader().Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // upgrader already wrote the error
	}
	defer ws.Close()
	done := make(chan struct{})
	defer close(done)
	go keepAlive(ws, done)

	execID, hr, err := dk(c).ExecTTY(c.Request.Context(), c.Param("id"), cmd)
	if err != nil {
		ws.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, err.Error()))
		return
	}
	defer hr.Close()

	// container -> browser
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := hr.Reader.Read(buf)
			if n > 0 {
				if werr := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				ws.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, "exited"))
				return
			}
		}
	}()

	// browser -> container
	for {
		kind, data, err := ws.ReadMessage()
		if err != nil {
			return
		}
		if kind == websocket.TextMessage && len(data) > 0 && data[0] == '{' {
			var msg struct {
				Resize []uint `json:"resize"`
			}
			if json.Unmarshal(data, &msg) == nil && len(msg.Resize) == 2 {
				_ = dk(c).ResizeExec(c.Request.Context(), execID, msg.Resize[0], msg.Resize[1])
				continue
			}
		}
		if _, err := hr.Conn.Write(data); err != nil {
			return
		}
	}
}
