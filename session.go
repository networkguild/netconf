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

	"github.com/networkguild/netconf/transport"
)

// ISession is definition of the operations that this netconf client provides
type ISession interface {
	SessionID() uint64
	ClientCapabilities() []string
	ServerCapabilities() []string
	HasCapability(string) bool
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

type sessionConfig struct {
	capabilities        []string
	notificationHandler NotificationHandler
	logger              Logger

	errSeverity []ErrSeverity
}

type SessionOption interface {
	apply(*sessionConfig)
}

type sessionOpt struct{ fn func(cfg *sessionConfig) }

func (o sessionOpt) apply(cl *sessionConfig) { o.fn(cl) }

// WithCapability sets supported client capabilities for the session
func WithCapability(capabilities ...string) SessionOption {
	return sessionOpt{func(cfg *sessionConfig) {
		cfg.capabilities = append(cfg.capabilities, capabilities...)
	}}
}

// WithNotificationHandler sets the notification handler for the session
func WithNotificationHandler(nh NotificationHandler) SessionOption {
	return sessionOpt{func(cfg *sessionConfig) {
		cfg.notificationHandler = nh
	}}
}

// WithLogger sets the logger for the session
func WithLogger(logger Logger) SessionOption {
	return sessionOpt{func(cfg *sessionConfig) {
		cfg.logger = logger
	}}
}

// WithErrorSeverity sets the severity level for errors returned by the server. Defaults are SevWarning, SevError
func WithErrorSeverity(severity ...ErrSeverity) SessionOption {
	return sessionOpt{func(cfg *sessionConfig) {
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
// A NotificationHandler function can be passed in as an option when calling Open method of Session object
// A typical use of the NotificationHandler function is to retrieve notifications once they are received so
// that they can be parsed and/or stored somewhere.
type NotificationHandler func(msg Notification)

// Open will create a new Session with th=e given transport and open it with the
// necessary hello messages.
func Open(transport transport.Transport, opts ...SessionOption) (ISession, error) {
	s := newSession(transport, opts...)

	if err := s.handshake(); err != nil {
		return nil, errors.Join(err, s.tr.Close())
	}

	go s.recv()
	return s, nil
}

func newSession(transport transport.Transport, opts ...SessionOption) *Session {
	cfg := sessionConfig{
		capabilities: DefaultCapabilities,
		logger:       &noOpLogger{},
		errSeverity:  []ErrSeverity{SevWarning, SevError},
	}

	for _, opt := range opts {
		opt.apply(&cfg)
	}

	s := &Session{
		tr:                  transport,
		clientCaps:          newCapabilitySet(cfg.capabilities...),
		reqs:                make(map[uint64]*req),
		notificationHandler: cfg.notificationHandler,
		errSeverity:         cfg.errSeverity,
		logger:              cfg.logger,
	}
	return s
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

	_, callErr := s.do(ctx, new(closeSession))

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

func (s *Session) handshake() error {
	r, err := s.tr.MsgReader()
	if err != nil {
		return err
	}
	defer r.Close()

	clientMsg := Hello{
		Capabilities: s.clientCaps.All(),
	}
	if err := s.writeMsg(&clientMsg); err != nil {
		return fmt.Errorf("failed to write hello message: %w", err)
	}

	var serverMsg Hello
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

	const baseCap11 = BaseCapability + ":1.1"
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

// HasCapability checks if server has a given capability.
func (s *Session) HasCapability(cap string) bool {
	return s.serverCaps.Has(cap)
}

type req struct {
	reply chan RpcReply
	ctx   context.Context
}

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

func (s *Session) recvMsg() error {
	r, err := s.tr.MsgReader()
	if err != nil {
		return err
	}
	defer func(r io.ReadCloser) {
		if err := r.Close(); err != nil {
			s.logger.Warnf("failed to close reader: %v", err)
		}
	}(r)

	buf := bytes.NewBuffer(make([]byte, 0, 8096))
	_, err = io.Copy(buf, r)
	if err != nil {
		return err
	}

	reply := buf.Bytes()
	switch {
	case bytes.Contains(reply, []byte("<rpc-reply")):
		rpcReply := RpcReply{rpc: reply}
		if err := xml.Unmarshal(reply, &rpcReply); err != nil {
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
			return fmt.Errorf("message %d context canceled: %s", rpcReply.MessageID, req.ctx.Err().Error())
		}
	case bytes.Contains(reply, []byte("<notification")):
		if s.notificationHandler == nil {
			s.logger.Warnf("Received notification but no handler is set")
			return nil
		}

		notif := Notification{rpc: reply}
		if err := xml.Unmarshal(reply, &notif); err != nil {
			return fmt.Errorf("failed to decode notification message: %w", err)
		}
		s.notificationHandler(notif)
	default:
		return fmt.Errorf("unknown rpc reply, notification and rpc-reply supported")
	}
	return nil
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
