// Package tunnel carries Docker API connections from the server to a
// `locci agent` over one outbound WebSocket, so hosts behind NAT need no open
// port. yamux multiplexes the WebSocket: the server opens a stream for every
// Docker connection, and the agent pipes each stream into its Docker socket.
package tunnel

import (
	"io"
	"net"
	"time"

	"github.com/gorilla/websocket"
)

// wsConn makes a WebSocket look like a net.Conn by carrying the byte stream
// in binary messages. yamux reads from one goroutine and writes from one
// goroutine, which is all a gorilla connection allows.
type wsConn struct {
	ws     *websocket.Conn
	reader io.Reader
}

// NewConn wraps an established WebSocket.
func NewConn(ws *websocket.Conn) net.Conn { return &wsConn{ws: ws} }

func (c *wsConn) Read(p []byte) (int, error) {
	for {
		if c.reader == nil {
			kind, r, err := c.ws.NextReader()
			if err != nil {
				return 0, err
			}
			if kind != websocket.BinaryMessage {
				continue
			}
			c.reader = r
		}
		n, err := c.reader.Read(p)
		if err == io.EOF {
			c.reader = nil
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}

func (c *wsConn) Write(p []byte) (int, error) {
	if err := c.ws.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *wsConn) Close() error                       { return c.ws.Close() }
func (c *wsConn) LocalAddr() net.Addr                { return c.ws.LocalAddr() }
func (c *wsConn) RemoteAddr() net.Addr               { return c.ws.RemoteAddr() }
func (c *wsConn) SetReadDeadline(t time.Time) error  { return c.ws.SetReadDeadline(t) }
func (c *wsConn) SetWriteDeadline(t time.Time) error { return c.ws.SetWriteDeadline(t) }
func (c *wsConn) SetDeadline(t time.Time) error {
	if err := c.ws.SetReadDeadline(t); err != nil {
		return err
	}
	return c.ws.SetWriteDeadline(t)
}
