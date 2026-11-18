package tunnel

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/hashicorp/yamux"
)

// Serve runs the agent side of a tunnel: every stream the server opens is
// piped to a fresh connection from dial (the local Docker socket). It returns
// when the tunnel closes.
func Serve(ctx context.Context, conn net.Conn, dial func(context.Context) (net.Conn, error), log *slog.Logger) error {
	sess, err := yamux.Server(conn, config())
	if err != nil {
		return err
	}
	defer sess.Close()
	go func() {
		<-ctx.Done()
		sess.Close()
	}()

	for {
		stream, err := sess.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		go func() {
			defer stream.Close()
			local, err := dial(ctx)
			if err != nil {
				log.Warn("agent: can't reach Docker", "err", err)
				return
			}
			defer local.Close()
			pipe(stream, local)
		}()
	}
}

// pipe copies both ways and returns when both directions are done, half
// closing so hijacked Docker connections (exec, attach) end cleanly.
func pipe(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	cp := func(dst, src net.Conn) {
		defer wg.Done()
		io.Copy(dst, src)
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			cw.CloseWrite()
		} else {
			dst.Close()
		}
	}
	go cp(a, b)
	go cp(b, a)
	wg.Wait()
}
