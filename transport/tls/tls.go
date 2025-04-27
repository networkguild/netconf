package tls

import (
	"context"
	"crypto/tls"
	"io"
	"net"

	"github.com/networkguild/netconf/internal"
	"github.com/networkguild/netconf/transport"
)

type framer = internal.Framer

// Transport implements RFC7589 for implementing NETCONF over TLS.
type Transport struct {
	*framer

	conn *tls.Conn

	managed bool
}

type Opt func(*Transport)

func WithDebugCapture(in io.Writer, out io.Writer) Opt {
	return func(t *Transport) {
		t.DebugCapture(in, out)
	}
}

// Dial will connect to a server via TLS and returns transport.Transport.
func Dial(ctx context.Context, network, addr string, config *tls.Config, opts ...Opt) (transport.Transport, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	tlsConn := tls.Client(conn, config)
	return newTransport(tlsConn, true, opts...), nil
}

// NewTransport takes an already connected tls transport and returns a new transport.Transport.
// The caller is responsible for closing underlying tls connection.
func NewTransport(conn *tls.Conn, opts ...Opt) (transport.Transport, error) {
	return newTransport(conn, false, opts...), nil
}

func newTransport(conn *tls.Conn, managed bool, opts ...Opt) *Transport {
	tr := &Transport{
		conn:    conn,
		framer:  internal.NewFramer(conn, conn),
		managed: managed,
	}

	for _, opt := range opts {
		opt(tr)
	}

	return tr
}

// Close will close the underlying transport
// Underlying TLS connection is closed if managed by transport (created by Dial).
func (t *Transport) Close() error {
	if t.managed {
		return t.conn.Close()
	}
	return nil
}
