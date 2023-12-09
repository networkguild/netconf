package netconf

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/networkguild/netconf/transport"
)

var ErrClosed = errors.New("closed connection")

type sessionConfig struct {
	capabilities        []string
	notificationHandler NotificationHandler
	logger              Logger
}

type SessionOption interface {
	apply(*sessionConfig)
}

type sessionOpt struct{ fn func(cfg *sessionConfig) }

func (o sessionOpt) apply(cl *sessionConfig) { o.fn(cl) }

func WithCapability(capabilities ...string) SessionOption {
	return sessionOpt{func(cfg *sessionConfig) {
		cfg.capabilities = append(cfg.capabilities, capabilities...)
	}}
}

func WithNotificationHandler(nh NotificationHandler) SessionOption {
	return sessionOpt{func(cfg *sessionConfig) {
		cfg.notificationHandler = nh
	}}
}

func WithLogger(logger Logger) SessionOption {
	return sessionOpt{func(cfg *sessionConfig) {
		cfg.logger = logger
	}}
}

// Session represents a netconf session to a one given device.
type Session struct {
	tr     transport.Transport
	logger Logger

	sessionID uint64
	seq       atomic.Uint64

	clientCaps          capabilitySet
	serverCaps          capabilitySet
	notificationHandler NotificationHandler

	mu      sync.Mutex
	reqs    map[uint64]*req
	closing bool
}

// NotificationHandler function allows to work with received notifications.
// A NotificationHandler function can be passed in as an option when calling Open method of Session object
// A typical use of the NotificationHandler function is to retrieve notifications once they are received so
// that they can be parsed and/or stored somewhere.
type NotificationHandler func(msg Notification)

// Open will create a new Session with th=e given transport and open it with the
// necessary hello messages.
func Open(transport transport.Transport, opts ...SessionOption) (*Session, error) {
	s := newSession(transport, opts...)

	// this needs a timeout of some sort.
	if err := s.handshake(); err != nil {
		s.tr.Close()
		return nil, err
	}

	go s.recv()
	return s, nil
}

func newSession(transport transport.Transport, opts ...SessionOption) *Session {
	cfg := sessionConfig{
		capabilities: DefaultCapabilities,
		logger:       &noOpLogger{},
	}

	for _, opt := range opts {
		opt.apply(&cfg)
	}

	s := &Session{
		tr:                  transport,
		clientCaps:          newCapabilitySet(cfg.capabilities...),
		reqs:                make(map[uint64]*req),
		notificationHandler: cfg.notificationHandler,
		logger:              cfg.logger,
	}
	return s
}

type hello struct {
	XMLName      xml.Name `xml:"urn:ietf:params:xml:ns:netconf:base:1.0 hello"`
	SessionID    uint64   `xml:"session-id,omitempty"`
	Capabilities []string `xml:"capabilities>capability"`
}

// handshake exchanges rpc hello messages and reports if there are any errors.
func (s *Session) handshake() error {
	r, err := s.tr.MsgReader()
	if err != nil {
		return err
	}
	defer r.Close()

	var serverMsg hello
	if err := xml.NewDecoder(r).Decode(&serverMsg); err != nil {
		return fmt.Errorf("failed to read server hello message: %w", err)
	}

	if serverMsg.SessionID == 0 {
		return fmt.Errorf("server did not return a session-id")
	}

	if len(serverMsg.Capabilities) == 0 {
		return fmt.Errorf("server did not return any capabilities")
	}

	s.serverCaps = newCapabilitySet(serverMsg.Capabilities...)
	s.sessionID = serverMsg.SessionID

	clientMsg := hello{
		Capabilities: s.clientCaps.All(),
	}
	if err := s.writeMsg(&clientMsg); err != nil {
		return fmt.Errorf("failed to write hello message: %w", err)
	}

	const baseCap11 = baseCap + ":1.1"
	if s.serverCaps.Has(baseCap11) && s.clientCaps.Has(baseCap11) {
		if upgrader, ok := s.tr.(interface{ Upgrade() }); ok {
			upgrader.Upgrade()
		}
	}

	return nil
}

// SessionID returns the current session ID exchanged in the hello messages.
// Will return 0 if there is no session ID.
func (s *Session) SessionID() uint64 {
	return s.sessionID
}

// ClientCapabilities will return the capabilities initialized with the session.
func (s *Session) ClientCapabilities() []string {
	return s.clientCaps.All()
}

// ServerCapabilities will return the capabilities returned by the server in
// its hello message.
func (s *Session) ServerCapabilities() []string {
	return s.serverCaps.All()
}

// startElement will walk through a xml.Decode until it finds a start element
// and returns it.
func startElement(d *xml.Decoder) (*xml.StartElement, error) {
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, err
		}

		if start, ok := tok.(xml.StartElement); ok {
			return &start, nil
		}
	}
}

type req struct {
	reply chan Reply
	ctx   context.Context
}

func (s *Session) recvMsg() error {
	r, err := s.tr.MsgReader()
	if err != nil {
		return err
	}
	defer r.Close()

	dec := xml.NewDecoder(r)
	root, err := startElement(dec)
	if err != nil {
		return err
	}

	switch root.Name {
	case NotificationName:
		if s.notificationHandler == nil {
			s.logger.Warnf("Received notification but no handler is set")
			return nil
		}

		var notif Notification
		if err := dec.DecodeElement(&notif, root); err != nil {
			return fmt.Errorf("failed to decode notification message: %w", err)
		}
		s.notificationHandler(notif)
	case RPCReplyName:
		var reply Reply
		if err := dec.DecodeElement(&reply, root); err != nil {
			return fmt.Errorf("failed to decode rpc-reply message: %w", err)
		}
		ok, req := s.req(reply.MessageID)
		if !ok {
			return fmt.Errorf("cannot find reply channel for message-id: %d", reply.MessageID)
		}

		select {
		case req.reply <- reply:
			return nil
		case <-req.ctx.Done():
			return fmt.Errorf("message %d context canceled: %s", reply.MessageID, req.ctx.Err().Error())
		}
	default:
		return fmt.Errorf("unknown message type: %q", root.Name.Local)
	}
	return nil
}

// recv is the main receive loop.  It runs concurrently to be able to handle
// interleaved messages (like notifications).
func (s *Session) recv() {
	for {
		err := s.recvMsg()
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
		if err != nil {
			s.logger.Errorf("failed to read incoming message, sessionId: %d, error: %v", s.sessionID, err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, req := range s.reqs {
		close(req.reply)
	}

	if !s.closing {
		s.logger.Errorf("connection closed unexpectedly, sessionId: %d", s.sessionID)
	}
}

func (s *Session) req(msgID uint64) (bool, *req) {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, ok := s.reqs[msgID]
	if !ok {
		for i, r := range s.reqs {
			if fb := s.seq.Load(); i == fb {
				delete(s.reqs, fb)
				return true, r
			}
		}
		return false, nil
	}
	delete(s.reqs, msgID)
	return true, req
}

func (s *Session) writeMsg(v any) error {
	w, err := s.tr.MsgWriter()
	if err != nil {
		return err
	}

	if err := xml.NewEncoder(w).Encode(v); err != nil {
		return err
	}
	return w.Close()
}

func (s *Session) send(ctx context.Context, msg *request) (chan Reply, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.writeMsg(msg); err != nil {
		return nil, err
	}

	ch := make(chan Reply, 1)
	s.reqs[msg.MessageID] = &req{
		reply: ch,
		ctx:   ctx,
	}

	return ch, nil
}

// Do issues a rpc call for the given NETCONF operation returning a Reply.  RPC
// errors (i.e. errors in the `<rpc-errors>` section of the `<rpc-reply>`) are
// converted into go errors automatically.  Instead use `reply.Err()` or
// `reply.RPCErrors` to access the errors and/or warnings.
func (s *Session) Do(ctx context.Context, req any) (*Reply, error) {
	msg := &request{
		MessageID: s.seq.Add(1),
		Operation: req,
	}

	ch, err := s.send(ctx, msg)
	if err != nil {
		return nil, err
	}

	select {
	case reply, ok := <-ch:
		if !ok {
			return nil, ErrClosed
		}
		if reply.Err() != nil {
			return nil, reply.Err()
		}
		return &reply, nil
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.reqs, msg.MessageID)
		s.mu.Unlock()

		return nil, ctx.Err()
	}
}

// Call issues a rpc message with `req` as the body and decodes the response into
// a pointer at `resp`.  Any Call errors are presented as a go error.
func (s *Session) Call(ctx context.Context, req any, resp any) error {
	reply, err := s.Do(ctx, &req)
	if err != nil {
		return err
	}

	if err := reply.Err(); err != nil {
		return err
	}

	if resp != nil {
		if err := reply.Decode(&resp); err != nil {
			return err
		}
	}

	return nil
}

// Close will gracefully close the sessions first by sending a `close-session`
// operation to the remote and then closing the underlying transport
func (s *Session) Close(ctx context.Context) error {
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()

	type closeSession struct {
		XMLName xml.Name `xml:"urn:ietf:params:xml:ns:netconf:base:1.0 close-session"`
	}

	_, callErr := s.Do(ctx, &closeSession{})

	if err := s.tr.Close(); err != nil &&
		!errors.Is(err, net.ErrClosed) &&
		!errors.Is(err, io.EOF) &&
		!errors.Is(err, syscall.EPIPE) {
		{
			return err
		}
	}

	if !errors.Is(callErr, io.EOF) &&
		!errors.Is(callErr, ErrClosed) {
		return callErr
	}

	return nil
}
