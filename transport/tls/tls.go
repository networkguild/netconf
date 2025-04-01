package tls

import (
	"context"
	"crypto/tls"
	"io"
	"net"

	"github.com/networkguild/netconf/internal"
)

// alias it to a private type, so we can make it private when embedding
type framer = internal.Framer

// Transport implements RFC7589 for implementing NETCONF over TLS.
type Transport struct {
	conn *tls.Conn

	*framer

	managed bool
}

type Opt func(*Transport)

func WithDebugCapture(in io.Writer, out io.Writer) Opt {
	return func(t *Transport) {
		t.framer.DebugCapture(in, out)
	}
}

// Dial will connect to a server via TLS and returns Transport.
func Dial(ctx context.Context, network, addr string, config *tls.Config, opts ...Opt) (*Transport, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	tlsConn := tls.Client(conn, config)
	return newTransport(tlsConn, true, opts...)

}

// NewTransport takes an already connected tls transport and returns a new
// Transport.
// The caller is responsible for closing underlying tls connection.
func NewTransport(conn *tls.Conn, opts ...Opt) (*Transport, error) {
	return newTransport(conn, false, opts...)
}

func newTransport(conn *tls.Conn, managed bool, opts ...Opt) (*Transport, error) {
	tr := &Transport{
		conn:    conn,
		framer:  internal.NewFramer(conn, conn),
		managed: managed,
	}

	for _, opt := range opts {
		opt(tr)
	}

	return tr, nil
}

// Close will close the underlying transport
// Underlying TLS connection is closed if managed by transport (created by Dial)
func (t *Transport) Close() error {
	if t.managed {
		return t.conn.Close()
	}
	return nil
}
