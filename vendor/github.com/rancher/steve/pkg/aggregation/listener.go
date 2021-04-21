package aggregation

import (
	"context"
	"io"
	"net"
	"sync"
)

type addr string

func (a addr) String() string {
	return string(a)
}
func (a addr) Network() string {
	return "tcp"
}

type Listener struct {
	sync.RWMutex

	address     addr
	connections chan net.Conn
	closed      bool
}

func NewListener(address string) *Listener {
	return &Listener{
		address:     addr(address),
		connections: make(chan net.Conn, 5),
	}
}

func (l *Listener) Accept() (net.Conn, error) {
	conn, ok := <-l.connections
	if !ok {
		return nil, io.ErrClosedPipe
	}
	return conn, nil
}

func (l *Listener) Close() error {
	l.Lock()
	defer l.Unlock()
	if !l.closed {
		close(l.connections)
		l.closed = true
	}
	return nil
}

func (l *Listener) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	left, right := net.Pipe()
	l.RLock()
	defer l.RUnlock()
	if l.closed {
		return nil, io.ErrClosedPipe
	}

	select {
	case l.connections <- right:
		return left, nil
	case <-ctx.Done():
		return nil, io.ErrClosedPipe
	}
}

func (l *Listener) Addr() net.Addr {
	return l.address
}
