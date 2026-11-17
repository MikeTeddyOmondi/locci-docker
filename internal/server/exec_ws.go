package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// The UI is served from the same origin; reject cross-site pages.
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == "" || strings.HasSuffix(origin, "://"+r.Host)
	},
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
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // upgrader already wrote the error
	}
	defer ws.Close()

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
