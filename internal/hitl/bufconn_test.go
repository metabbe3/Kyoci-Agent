package hitl

import (
	"net"
	"sync"
)

// bufconnListener is a minimal in-process net.Listener for unit tests.
// It exposes a Dial() method that returns the other end of an in-process
// pipe, letting a gRPC client and server talk without binding a real port.
//
// This avoids pulling google.golang.org/grpc/test/grpc_testing or bufconn as
// a dep — we only need bidirectional io pipe semantics for the round-trip test.
type bufconnListener struct {
	mu    sync.Mutex
	conns chan net.Conn
	done  chan struct{}
	addr  bufconnAddr
}

type bufconnAddr struct{}

func (bufconnAddr) Network() string { return "bufconn" }
func (bufconnAddr) String() string  { return "bufconn" }

func newBufconn(capacity int) net.Listener {
	return &bufconnListener{
		conns: make(chan net.Conn, capacity),
		done:  make(chan struct{}),
	}
}

func (l *bufconnListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *bufconnListener) Close() error {
	select {
	case <-l.done:
		// already closed
	default:
		close(l.done)
	}
	return nil
}

func (l *bufconnListener) Addr() net.Addr { return l.addr }

// Dial returns the client end of a new in-process pipe. The Accept() side
// gets the server end. Both ends are buffered; writes block when the buffer
// is full, exactly like a TCP socket.
func (l *bufconnListener) Dial() (net.Conn, error) {
	c1, c2 := net.Pipe()
	select {
	case l.conns <- c2:
		return c1, nil
	case <-l.done:
		c1.Close()
		c2.Close()
		return nil, net.ErrClosed
	}
}
