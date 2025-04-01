package ssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/networkguild/netconf/internal"
	"golang.org/x/crypto/ssh"
)

// alias it to a private type, so we can make it private when embedding
type framer = internal.Framer

// Transport implements RFC6242 for implementing NETCONF protocol over SSH.
type Transport struct {
	*framer

	stdin io.WriteCloser
	c     *ssh.Client
	sess  *ssh.Session

	managed bool
}

type Opt func(*Transport)

func WithDebugCapture(in io.Writer, out io.Writer) Opt {
	return func(t *Transport) {
		t.framer.DebugCapture(in, out)
	}
}

// Dial will connect to ssh server and issues a transport, it's used as a
// convenience function as essentially is the same as
//
//		c, err := ssh.Dial(network, addr, config)
//	 	if err != nil { /* ... handle error ... */ }
//	 	t, err := NewTransport(c)
//
// When the transport is closed the underlying connection is also closed.
func Dial(ctx context.Context, network, addr string, config *ssh.ClientConfig, opts ...Opt) (*Transport, error) {
	d := net.Dialer{Timeout: config.Timeout}
	conn, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	close(done)

	client := ssh.NewClient(sshConn, chans, reqs)
	return newTransport(client, true, opts...)
}

// NewTransport will create a new ssh transport as defined in RFC6242 for use
// with netconf.  Unlike Dial, the underlying client will not be automatically
// closed when the transport is closed (however any sessions and subsystems
// are still closed).
func NewTransport(client *ssh.Client, opts ...Opt) (*Transport, error) {
	return newTransport(client, false, opts...)
}

func newTransport(client *ssh.Client, managed bool, opts ...Opt) (*Transport, error) {
	sess, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create ssh session: %w", err)
	}

	w, err := sess.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	r, err := sess.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	const subsystem = "netconf"
	if err := sess.RequestSubsystem(subsystem); err != nil {
		return nil, fmt.Errorf("failed to start netconf ssh subsytem: %w", err)
	}

	tr := &Transport{
		framer:  internal.NewFramer(r, w),
		c:       client,
		sess:    sess,
		stdin:   w,
		managed: managed,
	}

	for _, opt := range opts {
		opt(tr)
	}

	return tr, nil
}

// Close will close the underlying transport.
// Underlying ssh.Client is closed if managed by transport (created by Dial)
func (t *Transport) Close() error {
	if t.managed {
		return errors.Join(t.stdin.Close(), t.sess.Close(), t.c.Close())
	} else {
		return errors.Join(t.stdin.Close(), t.sess.Close())
	}
}
