package netconf

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

type ExtantBool bool

func (b ExtantBool) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if !b {
		return nil
	}
	// This produces a empty start/end tag (i.e <tag></tag>) vs a self-closing
	// tag (<tag/>() which should be the same in XML, however I know certain
	// vendors may have issues with this format. We may have to process this
	// after xml encoding.
	//
	// See https://github.com/golang/go/issues/21399
	// or https://github.com/golang/go/issues/26756 for a different hack.
	return e.EncodeElement(struct{}{}, start)
}

func (b *ExtantBool) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	v := &struct{}{}
	if err := d.DecodeElement(v, &start); err != nil {
		return err
	}
	*b = v != nil
	return nil
}

type OKResp struct {
	OK ExtantBool `xml:"ok"`
}

type Datastore string

func (s Datastore) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if s == "" {
		return fmt.Errorf("datastores cannot be empty")
	}

	// XXX: it would be nice to actually just block names with crap in them
	// instead of escaping them, but we need to find a list of what is allowed
	// in an xml tag.
	escaped, err := escapeXML(string(s))
	if err != nil {
		return fmt.Errorf("invalid string element: %w", err)
	}

	v := struct {
		Elem string `xml:",innerxml"`
	}{Elem: "<" + escaped + "/>"}
	return e.EncodeElement(&v, start)
}

func escapeXML(input string) (string, error) {
	buf := &strings.Builder{}
	if err := xml.EscapeText(buf, []byte(input)); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type URL string

func (u URL) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	v := struct {
		URL string `xml:"url"`
	}{string(u)}
	return e.EncodeElement(&v, start)
}

const (
	// Running configuration datastore. Required by RFC6241
	Running Datastore = "running"

	// Candidate configuration configuration datastore.  Supported with the
	// `:candidate` capability defined in RFC6241 section 8.3
	Candidate Datastore = "candidate"

	// Startup configuration configuration datastore.  Supported with the
	// `:startup` capability defined in RFC6241 section 8.7
	Startup Datastore = "startup" //
)

type GetConfigReq struct {
	XMLName xml.Name  `xml:"urn:ietf:params:xml:ns:netconf:base:1.0 get-config"`
	Source  Datastore `xml:"source"`
	Filter  *Filter   `xml:"filter,omitempty"`
}

// GetConfig implements the <get-config> rpc operation defined in [RFC6241 7.1].
// `source` is the datastore to query.
//
// [RFC6241 7.1]: https://www.rfc-editor.org/rfc/rfc6241.html#section-7.1
func (s *Session) GetConfig(ctx context.Context, source Datastore) (*Reply, error) {
	req := GetConfigReq{
		Source: source,
	}

	return s.Do(ctx, &req)
}

type GetReq struct {
	XMLName      xml.Name     `xml:"urn:ietf:params:xml:ns:netconf:base:1.0 get"`
	Filter       *Filter      `xml:"filter,omitempty"`
	WithDefaults DefaultsMode `xml:"urn:ietf:params:xml:ns:yang:ietf-netconf-with-defaults with-defaults,omitempty"`
}

type Filter struct {
	Type  string `xml:"type,attr,omitempty"`
	Inner any    `xml:",innerxml"`
}

// DefaultsMode defines the strategies for merging configuration in a
// `<edit-config> operation`.
//
// *Note*: in RFC6241 7.2 this is called the `operation` attribute and
// `default-operation` parameter.  Since the `operation` term is already
// overloaded this was changed to `MergeStrategy` for a cleaner API.
type DefaultsMode string

type withDefault DefaultsMode

func (o withDefault) apply(req *GetReq) { req.WithDefaults = DefaultsMode(o) }

// WithDefaultMode sets the `with-defaults` in the `<get>` or `<get-config>` operation.
// This defines the behavior if default configs should be returned.
// See [DefaultsMode] for the available options.
func WithDefaultMode(op DefaultsMode) GetOption { return withDefault(op) }

type GetOption interface {
	apply(*GetReq)
}

// Get issues the `<get>` operation as defined in [RFC6241 7.7]
// for retrieving running configuration and device state information.
//
// Only the `subtree` filter type is supported.
//
// [RFC6241 7.7] https://www.rfc-editor.org/rfc/rfc6241.html#section-7.7
func (s *Session) Get(ctx context.Context, filter any, opts ...GetOption) (*Reply, error) {
	const filterType = "subtree"
	var req GetReq

	switch v := filter.(type) {
	case string:
		req.Filter = &Filter{
			Type:  filterType,
			Inner: []byte(v),
		}
	case []byte:
		req.Filter = &Filter{
			Type:  filterType,
			Inner: v,
		}
	default:
		req.Filter = &Filter{
			Type:  filterType,
			Inner: v,
		}
	}

	for _, opt := range opts {
		opt.apply(&req)
	}

	reply, err := s.Do(ctx, &req)
	if err != nil {
		return nil, err
	}

	return reply, nil
}

// MergeStrategy defines the strategies for merging configuration in a
// `<edit-config> operation`.
//
// *Note*: in RFC6241 7.2 this is called the `operation` attribute and
// `default-operation` parameter.  Since the `operation` term is already
// overloaded this was changed to `MergeStrategy` for a cleaner API.
type MergeStrategy string

const (
	// MergeConfig configuration elements are merged together at the level at
	// which this specified.  Can be used for config elements as well as default
	// defined with [WithDefaultMergeStrategy] option.
	MergeConfig MergeStrategy = "merge"

	// ReplaceConfig defines that the incoming config change should replace the
	// existing config at the level which it is specified.  This can be
	// specified on individual config elements or set as the default strategy set
	// with [WithDefaultMergeStrategy] option.
	ReplaceConfig MergeStrategy = "replace"

	// NoMergeStrategy is only used as a default strategy defined in
	// [WithDefaultMergeStrategy].  Elements must specific one of the other
	// strategies with the `operation` Attribute on elements in the `<config>`
	// subtree.  Elements without the `operation` attribute are ignored.
	NoMergeStrategy MergeStrategy = "none"

	// CreateConfig allows a subtree element to be created only if it doesn't
	// already exist.
	// This strategy is only used as the `operation` attribute of
	// a `<config>` element and cannot be used as the default strategy.
	CreateConfig MergeStrategy = "create"

	// DeleteConfig will completely delete subtree from the config only if it
	// already exists.  This strategy is only used as the `operation` attribute
	// of a `<config>` element and cannot be used as the default strategy.
	DeleteConfig MergeStrategy = "delete"

	// RemoveConfig will remove subtree from the config.  If the subtree doesn't
	// exist in the datastore then it is silently skipped.  This strategy is
	// only used as the `operation` attribute of a `<config>` element and cannot
	// be used as the default strategy.
	RemoveConfig MergeStrategy = "remove"
)

// TestStrategy defines the behavior for testing configuration before applying it in a `<edit-config>` operation.
//
// *Note*: in RFC6241 7.2 this is called the `test-option` parameter. Since the `option` term is already
// overloaded this was changed to `TestStrategy` for a cleaner API.
type TestStrategy string

const (
	// TestThenSet will validate the configuration and only if is is valid then
	// apply the configuration to the datastore.
	TestThenSet TestStrategy = "test-then-set"

	// SetOnly will not do any testing before applying it.
	SetOnly TestStrategy = "set"

	// Test only will validation the incoming configuration and return the
	// results without modifying the underlying store.
	TestOnly TestStrategy = "test-only"
)

// ErrorStrategy defines the behavior when an error is encountered during a `<edit-config>` operation.
//
// *Note*: in RFC6241 7.2 this is called the `error-option` parameter. Since the `option` term is already
// overloaded this was changed to `ErrorStrategy` for a cleaner API.
type ErrorStrategy string

const (
	// StopOnError will about the `<edit-config>` operation on the first error.
	StopOnError ErrorStrategy = "stop-on-error"

	// ContinueOnError will continue to parse the configuration data even if an
	// error is encountered.  Errors are still recorded and reported in the
	// reply.
	ContinueOnError ErrorStrategy = "continue-on-error"

	// RollbackOnError will restore the configuration back to before the
	// `<edit-config>` operation took place.  This requires the device to
	// support the `:rollback-on-error` capabilitiy.
	RollbackOnError ErrorStrategy = "rollback-on-error"
)

type (
	defaultMergeStrategy MergeStrategy
	testStrategy         TestStrategy
	errorStrategy        ErrorStrategy
)

func (o defaultMergeStrategy) apply(req *EditConfigReq) { req.DefaultMergeStrategy = MergeStrategy(o) }
func (o testStrategy) apply(req *EditConfigReq)         { req.TestStrategy = TestStrategy(o) }
func (o errorStrategy) apply(req *EditConfigReq)        { req.ErrorStrategy = ErrorStrategy(o) }

// WithDefaultMergeStrategy sets the default config merging strategy for the
// <edit-config> operation.  Only [Merge], [Replace], and [None] are supported
// (the rest of the strategies are for defining as attributed in individual
// elements inside the `<config>` subtree).
func WithDefaultMergeStrategy(op MergeStrategy) EditConfigOption { return defaultMergeStrategy(op) }

// WithTestStrategy sets the `test-option` in the `<edit-config>“ operation.
// This defines what testing should be done the supplied configuration.  See the
// documentation on [TestStrategy] for details on each strategy.
func WithTestStrategy(op TestStrategy) EditConfigOption { return testStrategy(op) }

// WithErrorStrategy sets the `error-option` in the `<edit-config>` operation.
// This defines the behavior when errors are encountered applying the supplied
// config.  See [ErrorStrategy] for the available options.
func WithErrorStrategy(opt ErrorStrategy) EditConfigOption { return errorStrategy(opt) }

type EditConfigReq struct {
	XMLName              xml.Name      `xml:"urn:ietf:params:xml:ns:netconf:base:1.0 edit-config"`
	Target               Datastore     `xml:"target"`
	DefaultMergeStrategy MergeStrategy `xml:"default-operation,omitempty"`
	TestStrategy         TestStrategy  `xml:"test-option,omitempty"`
	ErrorStrategy        ErrorStrategy `xml:"error-option,omitempty"`

	Inner any    `xml:",innerxml"`
	URL   string `xml:"url,omitempty"`
}

// EditConfigOption is a optional arguments to [Session.EditConfig] method
type EditConfigOption interface {
	apply(*EditConfigReq)
}

const (
	configPrefix = "<config"
	configSuffix = "</config>"
)

// EditConfig issues the `<edit-config>` operation defined in [RFC6241 7.2] for
// updating an existing target config datastore.
//
// [RFC6241 7.2]: https://www.rfc-editor.org/rfc/rfc6241.html#section-7.2
func (s *Session) EditConfig(ctx context.Context, target Datastore, config any, opts ...EditConfigOption) (*Reply, error) {
	req := EditConfigReq{
		Target: target,
	}

	switch v := config.(type) {
	case string:
		if !strings.HasPrefix(v, configPrefix) {
			v = fmt.Sprintf("%s\n%s\n%s", configPrefix+">", v, configSuffix)
		}
		req.Inner = []byte(v)
	case []byte:
		if !bytes.HasPrefix(v, []byte(configPrefix)) {
			v = []byte(fmt.Sprintf("%s\n%s\n%s", configPrefix+">", v, configSuffix))
		}
		req.Inner = v
	case URL:
		req.URL = string(v)
	default:
		req.Inner = v
	}

	for _, opt := range opts {
		opt.apply(&req)
	}

	return s.Do(ctx, &req)
}

type CopyConfigReq struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:netconf:base:1.0 copy-config"`
	Target  any      `xml:"target"`
	Source  any      `xml:"source"`
}

// CopyConfig issues the `<copy-config>` operation as defined in [RFC6241 7.3]
// for copying an entire config to/from a source and target datastore.
//
// A `<config>` element defining a full config can be used as the source.
//
// If a device supports the `:url` capability than a [URL] object can be used
// for the source or target datastore.
//
// [RFC6241 7.3] https://www.rfc-editor.org/rfc/rfc6241.html#section-7.3
func (s *Session) CopyConfig(ctx context.Context, source, target any) (*Reply, error) {
	req := CopyConfigReq{
		Source: source,
		Target: target,
	}

	return s.Do(ctx, &req)
}

type DeleteConfigReq struct {
	XMLName xml.Name  `xml:"urn:ietf:params:xml:ns:netconf:base:1.0 delete-config"`
	Target  Datastore `xml:"target"`
}

// DeleteConfig issues the `<delete-config>` operation as defined in [RFC6241 7.4]
// for deleting a configuration datastore.
//
// [RFC6241 7.4] https://www.rfc-editor.org/rfc/rfc6241.html#section-7.4
func (s *Session) DeleteConfig(ctx context.Context, target Datastore) (*Reply, error) {
	req := DeleteConfigReq{
		Target: target,
	}

	return s.Do(ctx, &req)
}

type LockReq struct {
	XMLName xml.Name
	Target  Datastore `xml:"target"`
}

// Lock issues the `<lock>` operation as defined in [RFC6241 7.5]
// for locking the entire configuration datastore.
//
// [RFC6241 7.5] https://www.rfc-editor.org/rfc/rfc6241.html#section-7.5
func (s *Session) Lock(ctx context.Context, target Datastore) (*Reply, error) {
	req := LockReq{
		XMLName: xml.Name{Space: "urn:ietf:params:xml:ns:netconf:base:1.0", Local: "lock"},
		Target:  target,
	}

	return s.Do(ctx, &req)
}

// Unlock issues the `<unlock>` operation as defined in [RFC6241 7.6]
// for releasing a configuration lock, previously obtained with the [Session.Lock] operation.
//
// [RFC6241 7.6] https://www.rfc-editor.org/rfc/rfc6241.html#section-7.6
func (s *Session) Unlock(ctx context.Context, target Datastore) (*Reply, error) {
	req := LockReq{
		XMLName: xml.Name{Space: "urn:ietf:params:xml:ns:netconf:base:1.0", Local: "unlock"},
		Target:  target,
	}

	return s.Do(ctx, &req)
}

type KillSessionReq struct {
	XMLName   xml.Name `xml:"urn:ietf:params:xml:ns:netconf:base:1.0 kill-session"`
	SessionID uint64   `xml:"session-id"`
}

// KillSession issues the `<kill-session>` operation as defined in [RFC6241 7.9]
// for force terminating the NETCONF session.
//
// [RFC6241 7.9] https://www.rfc-editor.org/rfc/rfc6241.html#section-7.9
func (s *Session) KillSession(ctx context.Context, sessionID uint64) (*Reply, error) {
	req := KillSessionReq{
		SessionID: sessionID,
	}

	return s.Do(ctx, &req)
}

type ValidateReq struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:netconf:base:1.0 validate"`
	Source  any      `xml:"source"`
}

// Validate issues the `<validate>` operation as defined in [RFC6241 8.6.4.1]
// for validating the contents of the specified configuration. This requires
// the device to support the [ValidateCapability] capability
//
// [RFC6241 8.6.4.1] https://www.rfc-editor.org/rfc/rfc6241.html#section-8.6.4.1
func (s *Session) Validate(ctx context.Context, source any) (*Reply, error) {
	if !s.serverCaps.Has(ValidateCapability) {
		return nil, fmt.Errorf("server does not support validate")
	}
	req := ValidateReq{
		Source: source,
	}

	return s.Do(ctx, &req)
}

type CommitReq struct {
	XMLName        xml.Name   `xml:"urn:ietf:params:xml:ns:netconf:base:1.0 commit"`
	Confirmed      ExtantBool `xml:"confirmed,omitempty"`
	ConfirmTimeout int64      `xml:"confirm-timeout,omitempty"`
	Persist        string     `xml:"persist,omitempty"`
	PersistID      string     `xml:"persist-id,omitempty"`
}

// CommitOption is a optional arguments to [Session.Commit] method
type CommitOption interface {
	apply(*CommitReq)
}

type confirmed bool
type confirmedTimeout struct {
	time.Duration
}
type persist string
type PersistID string

func (o confirmed) apply(req *CommitReq) { req.Confirmed = true }
func (o confirmedTimeout) apply(req *CommitReq) {
	req.Confirmed = true
	req.ConfirmTimeout = int64(o.Seconds())
}
func (o persist) apply(req *CommitReq) {
	req.Confirmed = true
	req.Persist = string(o)
}
func (o PersistID) apply(req *CommitReq) { req.PersistID = string(o) }

// RollbackOnError will restore the configuration back to before the
// `<edit-config>` operation took place.  This requires the device to
// support the `:rollback-on-error` capability.

// WithConfirmed will mark the commits as requiring confirmation or will rollback
// after the default timeout on the device (default should be 600s).  The commit
// can be confirmed with another `<commit>` call without the confirmed option,
// extended by calling with `Commit` With `WithConfirmed` or
// `WithConfirmedTimeout` or canceling the commit with a `CommitCancel` call.
// This requires the device to support the `:confirmed-commit:1.1` capability.
func WithConfirmed() CommitOption { return confirmed(true) }

// WithConfirmedTimeout is like `WithConfirmed` but using the given timeout
// duration instead of the device's default.
func WithConfirmedTimeout(timeout time.Duration) CommitOption { return confirmedTimeout{timeout} }

// WithPersist allows you to set a identifier to confirm a commit in another
// sessions.  Confirming the commit requires setting the `WithPersistID` in the
// following `Commit` call matching the id set on the confirmed commit.  Will
// mark the commit as confirmed if not already set.
func WithPersist(id string) CommitOption { return persist(id) }

// WithPersistID is used to confirm a previous commit set with a given
// identifier.  This allows you to confirm a commit from (potentially) another
// session.
func WithPersistID(id string) PersistID { return PersistID(id) }

// Commit will commit a candidate config to the running config. This requires
// the device to support the `:candidate` capability.
func (s *Session) Commit(ctx context.Context, opts ...CommitOption) (*Reply, error) {
	var req CommitReq
	for _, opt := range opts {
		opt.apply(&req)
	}

	if req.PersistID != "" && req.Confirmed {
		return nil, fmt.Errorf("PersistID cannot be used with Confirmed/ConfirmedTimeout or Persist options")
	}

	return s.Do(ctx, &req)
}

// CancelCommitOption is a optional arguments to [Session.CancelCommit] method
type CancelCommitOption interface {
	applyCancelCommit(*CancelCommitReq)
}

func (o PersistID) applyCancelCommit(req *CancelCommitReq) { req.PersistID = string(o) }

type CancelCommitReq struct {
	XMLName   xml.Name `xml:"urn:ietf:params:xml:ns:netconf:base:1.0 cancel-commit"`
	PersistID string   `xml:"persist-id,omitempty"`
}

func (s *Session) CancelCommit(ctx context.Context, opts ...CancelCommitOption) (*Reply, error) {
	var req CancelCommitReq
	for _, opt := range opts {
		opt.applyCancelCommit(&req)
	}

	return s.Do(ctx, &req)
}

// CreateSubscriptionOption is a optional arguments to [Session.CreateSubscription] method
type CreateSubscriptionOption interface {
	apply(req *CreateSubscriptionReq)
}

type CreateSubscriptionReq struct {
	XMLName   xml.Name `xml:"urn:ietf:params:xml:ns:netconf:notification:1.0 create-subscription"`
	Stream    string   `xml:"stream,omitempty"`
	Filter    *Filter  `xml:"filter,omitempty"`
	StartTime string   `xml:"startTime,omitempty"`
	EndTime   string   `xml:"endTime,omitempty"`
}

type stream string
type startTime time.Time
type endTime time.Time

func (o stream) apply(req *CreateSubscriptionReq) {
	req.Stream = string(o)
}
func (o startTime) apply(req *CreateSubscriptionReq) {
	req.StartTime = time.Time(o).Format(time.RFC3339)
}
func (o endTime) apply(req *CreateSubscriptionReq) {
	req.EndTime = time.Time(o).Format(time.RFC3339)
}

func WithStreamOption(s string) CreateSubscriptionOption        { return stream(s) }
func WithStartTimeOption(st time.Time) CreateSubscriptionOption { return startTime(st) }
func WithEndTimeOption(et time.Time) CreateSubscriptionOption   { return endTime(et) }

// CreateSubscription issues the `<create-subscription>` operation as defined in [RFC5277 2.1.1]
// for initiating an event notification subscription that will send asynchronous event notifications to the initiator.
//
// This requires the device to support the [NotificationCapability] capability
//
// [RFC5277 2.1.1] https://www.rfc-editor.org/rfc/rfc5277.html#section-2.1.1
func (s *Session) CreateSubscription(ctx context.Context, opts ...CreateSubscriptionOption) (*Reply, error) {
	if !s.serverCaps.Has(NotificationCapability) {
		return nil, fmt.Errorf("server does not support notifications")
	}
	var req CreateSubscriptionReq
	for _, opt := range opts {
		opt.apply(&req)
	}

	return s.Do(ctx, &req)
}

// Dispatch issues custom `<rpc>` operation
func (s *Session) Dispatch(ctx context.Context, rpc any) (*Reply, error) {
	return s.Do(ctx, &rpc)
}
