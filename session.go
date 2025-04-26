package netconf

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/networkguild/netconf/transport"
)

var pool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 8096))
	},
}

// ISession is definition of the operations that this netconf client provides
type ISession interface {
	SessionID() uint64
	ClientCapabilities() []string
	ServerCapabilities() []string
	HasCapability(string) bool
	Logger() Logger
	GetConfig(context.Context, Datastore, ...GetOption) (*RpcReply, error)
	Get(context.Context, ...GetOption) (*RpcReply, error)
	EditConfig(context.Context, Datastore, any, ...EditConfigOption) error
	DiscardChanges(context.Context) error
	CopyConfig(context.Context, any, any) error
	DeleteConfig(context.Context, Datastore) error
	Lock(context.Context, Datastore) error
	Unlock(context.Context, Datastore) error
	KillSession(context.Context, uint64) (*RpcReply, error)
	Close(context.Context) error
	Validate(context.Context, any) error
	Commit(context.Context, ...CommitOption) error
	CancelCommit(context.Context, ...CancelCommitOption) error
	Dispatch(context.Context, any) (*RpcReply, error)
	DispatchWithReply(context.Context, any, any) error
	CreateSubscription(context.Context, ...CreateSubscriptionOption) error
}

var ErrClosed = errors.New("closed connection")

type SessionOption interface {
	apply(*Session)
}

type sessionOpt struct{ fn func(cfg *Session) }

func (o sessionOpt) apply(cl *Session) { o.fn(cl) }

// WithTransport sets the transport for the session
func WithTransport(transport transport.Transport) SessionOption {
	return sessionOpt{func(cfg *Session) {
		cfg.tr = transport
	}}
}

// WithCapabilities sets supported client capabilities for the session
func WithCapabilities(capabilities ...string) SessionOption {
	return sessionOpt{func(cfg *Session) {
		cfg.clientCaps = newCapabilitySet(capabilities...)
	}}
}

// WithNotificationHandler sets the notification handler for the session
func WithNotificationHandler(nh NotificationHandler) SessionOption {
	return sessionOpt{func(cfg *Session) {
		cfg.notificationHandler = nh
	}}
}

// WithLogger sets the logger for the session
func WithLogger(logger Logger) SessionOption {
	return sessionOpt{func(cfg *Session) {
		cfg.logger = logger
	}}
}

// WithErrorSeverity sets the severity level for errors returned by the server. Defaults are SevWarning, SevError
func WithErrorSeverity(severity ...ErrSeverity) SessionOption {
	return sessionOpt{func(cfg *Session) {
		cfg.errSeverity = severity
	}}
}

// Session represents a netconf session to a one given device.
type Session struct {
	tr     transport.Transport
	logger Logger

	sessionID uint64
	seq       atomic.Uint64

	clientCaps capabilitySet
	serverCaps capabilitySet

	notificationHandler NotificationHandler
	errSeverity         []ErrSeverity

	mu      sync.Mutex
	reqs    map[uint64]*req
	closing bool
}

// NotificationHandler function allows to work with received notifications.
// A NotificationHandler function can be passed in as an option when calling NewSession method of Session object
// Typical use of the NotificationHandler function is to retrieve notifications once they are received so
// that they can be parsed and/or stored somewhere.
type NotificationHandler func(msg Notification)

// NewSession will create a new Session with th=e given transport and open it with the
// necessary hello messages.
// WithTransport SessionOption is required to set the transport for the session.
func NewSession(ctx context.Context, opts ...SessionOption) (ISession, error) {
	s, err := newSession(opts...)
	if err != nil {
		return nil, err
	}

	if err := s.handshake(ctx); err != nil {
		return nil, errors.Join(err, s.tr.Close())
	}

	go s.recv()
	return s, nil
}

func newSession(opts ...SessionOption) (*Session, error) {
	sess := Session{
		clientCaps: newCapabilitySet(DefaultCapabilities...),
		reqs:       make(map[uint64]*req),
		logger:     &noOpLogger{},
	}

	for _, opt := range opts {
		opt.apply(&sess)
	}

	if sess.tr == nil {
		return nil, errors.New("transport is required for session")
	}

	return &sess, nil
}

type CloseSessionRequest struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:netconf:base:1.0 close-session"`
}

// Close will gracefully close the sessions first by sending a `close-session`
// operation to the remote and then closing the underlying transport
func (s *Session) Close(ctx context.Context) error {
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()

	_, callErr := s.do(ctx, new(CloseSessionRequest))

	err := s.tr.Close()
	if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) && !errors.Is(err, syscall.EPIPE) {
		return err
	}

	if !errors.Is(callErr, io.EOF) && !errors.Is(callErr, ErrClosed) {
		return callErr
	}

	return nil
}

func (s *Session) handshake(ctx context.Context) error {
	clientMsg := Hello{
		Capabilities: s.clientCaps.All(),
	}
	if err := s.writeMsg(&clientMsg); err != nil {
		return fmt.Errorf("failed to write hello message: %w", err)
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- s.recvMsg()
	}()

	select {
	case <-time.After(30 * time.Second):
		return errors.New("timeout waiting for hello")
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errChan:
		return err
	}
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

// HasCapability checks if server has a given capability.
func (s *Session) HasCapability(cap string) bool {
	return s.serverCaps.Has(cap)
}

func (s *Session) Logger() Logger {
	if s.logger == nil {
		return &noOpLogger{}
	}
	return s.logger
}

type req struct {
	reply chan RpcReply
	ctx   context.Context
}

func (s *Session) recv() {
	for {
		if err := s.recvMsg(); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			s.logger.Errorf("failed to read incoming message, sessionId: %d, error: %v", s.sessionID, err)
		}
	}

	s.mu.Lock()
	for _, req := range s.reqs {
		close(req.reply)
	}
	s.mu.Unlock()

	if !s.closing {
		s.logger.Errorf("connection closed unexpectedly, sessionId: %d", s.sessionID)
	}
}

func (s *Session) recvMsg() error {
	raw, err := s.readWithPoolBuffer()
	if err != nil {
		return err
	}

	var elem *xml.StartElement
	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}

		if start, ok := tok.(xml.StartElement); ok {
			elem = &start
			break
		}
	}

	switch elem.Name.Local {
	case "rpc-reply":
		rpcReply := RpcReply{rpc: raw}
		if err := dec.DecodeElement(&rpcReply, elem); err != nil {
			return fmt.Errorf("failed to decode rpc-reply message: %w", err)
		}
		ok, req := s.req(rpcReply.MessageID)
		if !ok {
			return fmt.Errorf("cannot find reply channel for message-id: %d", rpcReply.MessageID)
		}

		select {
		case req.reply <- rpcReply:
			return nil
		case <-req.ctx.Done():
			return fmt.Errorf("message %d context canceled: %w", rpcReply.MessageID, req.ctx.Err())
		}
	case "notification":
		notif := Notification{rpc: raw}
		if err := dec.DecodeElement(&notif, elem); err != nil {
			return fmt.Errorf("failed to decode notification message: %w", err)
		}
		s.notificationHandler(notif)
		return nil
	case "hello":
		var hello Hello
		if err := dec.DecodeElement(&hello, elem); err != nil {
			return fmt.Errorf("failed to decode hello message: %w", err)
		}
		if hello.SessionID == 0 {
			return errors.New("server did not return a session-id")
		}

		if len(hello.Capabilities) == 0 {
			return errors.New("server did not return any capabilities")
		}

		s.serverCaps = newCapabilitySet(hello.Capabilities...)
		s.sessionID = hello.SessionID

		const baseCap11 = BaseCapability + ":1.1"
		if s.serverCaps.Has(baseCap11) && s.clientCaps.Has(baseCap11) {
			if upgrader, ok := s.tr.(interface{ Upgrade() }); ok {
				upgrader.Upgrade()
			}
		}
		return nil
	default:
		s.logger.Warnf("unsupported message type '%s' received; only 'hello', 'rpc-reply' and 'notification' messages are supported", elem.Name.Local)
		return nil
	}
}

func (s *Session) readWithPoolBuffer() ([]byte, error) {
	r, err := s.tr.MsgReader()
	if err != nil {
		return nil, err
	}
	defer r.Close()

	buf := pool.Get().(*bytes.Buffer)
	defer pool.Put(buf)
	buf.Reset()

	_, err = io.Copy(buf, r)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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

func (s *Session) call(ctx context.Context, req any, resp any) error {
	reply, err := s.do(ctx, req)
	if err != nil {
		return err
	}

	if err := reply.Err(); err != nil {
		return err
	}

	if resp != nil {
		if err := reply.Decode(resp); err != nil {
			return err
		}
	}

	return nil
}

func (s *Session) do(ctx context.Context, req any) (*RpcReply, error) {
	msg := &Rpc{
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
		if reply.Err(s.errSeverity...) != nil {
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

func (s *Session) send(ctx context.Context, msg *Rpc) (chan RpcReply, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.writeMsg(msg); err != nil {
		return nil, err
	}

	ch := make(chan RpcReply, 1)
	s.reqs[msg.MessageID] = &req{
		reply: ch,
		ctx:   ctx,
	}

	return ch, nil
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
