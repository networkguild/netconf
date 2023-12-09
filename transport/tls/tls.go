package tls

import (
	"context"
	"crypto/tls"
	"net"

	"github.com/networkguild/netconf/transport"
)

// alias it to a private type, so we can make it private when embedding
type framer = transport.Framer

// Transport implements RFC7589 for implementing NETCONF over TLS.
type Transport struct {
	conn *tls.Conn

	*framer

	managedByTransport bool
}

// Dial will connect to a server via TLS and returns Transport.
func Dial(ctx context.Context, network, addr string, config *tls.Config) (*Transport, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	tlsConn := tls.Client(conn, config)
	return newTransport(tlsConn, true), nil

}

// NewTransport takes an already connected tls transport and returns a new
// Transport.
// The caller is responsible for closing underlying tls connection.
func NewTransport(conn *tls.Conn) *Transport {
	return newTransport(conn, false)
}

func newTransport(conn *tls.Conn, managed bool) *Transport {
	return &Transport{
		conn:               conn,
		framer:             transport.NewFramer(conn, conn),
		managedByTransport: managed,
	}
}

// Close will close the underlying transport
// Underlying TLS connection is closed if managed by transport (created by Dial)
func (t *Transport) Close() error {
	if t.managedByTransport {
		return t.conn.Close()
	}
	return nil
}
