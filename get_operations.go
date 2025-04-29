package netconf

import (
	"context"
	"encoding/xml"
	"fmt"
)

func (f Filter) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	v := struct {
		Type   string `xml:"type,attr,omitempty"`
		Filter string `xml:",innerxml"`
	}{Type: "subtree", Filter: string(f)}
	return e.EncodeElement(&v, start)
}

type GetRequest struct {
	XMLName      xml.Name
	Attr         []xml.Attr   `xml:",attr,omitempty"`
	Source       *Datastore   `xml:"source,omitempty"`
	Filter       Filter       `xml:"filter,omitempty"`
	WithDefaults DefaultsMode `xml:"urn:ietf:params:xml:ns:yang:ietf-netconf-with-defaults with-defaults,omitempty"`
}

func (r *GetRequest) validate(cap capabilitySet) error {
	if r.WithDefaults != "" {
		isValid := cap.ContainsValue(WithDefaultsCapability, "basic-mode", string(r.WithDefaults)) ||
			cap.ContainsValue(WithDefaultsCapability, "also-supported", string(r.WithDefaults))
		if !isValid {
			return fmt.Errorf("unsupport with defaults value: %s", r.WithDefaults)
		}
	}
	return nil
}

func NewGetConfigRequest(source Datastore, opts ...GetOption) *GetRequest {
	req := GetRequest{
		XMLName: xml.Name{Space: baseNetconfNs, Local: "get-config"},
		Source:  &source,
	}

	for _, opt := range opts {
		opt.apply(&req)
	}

	return &req
}

func NewGetRequest(opts ...GetOption) *GetRequest {
	req := GetRequest{
		XMLName: xml.Name{Space: baseNetconfNs, Local: "get"},
	}

	for _, opt := range opts {
		opt.apply(&req)
	}

	return &req
}

type GetResponse struct {
	Data []byte `xml:"data"`
}

type Filter string

type DefaultsMode string

const (
	DefaultsModeTrim           DefaultsMode = "trim"
	DefaultModeReportAll       DefaultsMode = "report-all"
	DefaultModeReportAllTagged DefaultsMode = "report-all-tagged"
	DefaultModeExplicit        DefaultsMode = "explicit"
)

type defaultsMode DefaultsMode

type filter Filter

type attr xml.Attr

func (o filter) apply(req *GetRequest)       { req.Filter = Filter(o) }
func (o defaultsMode) apply(req *GetRequest) { req.WithDefaults = DefaultsMode(o) }
func (o attr) apply(req *GetRequest) {
	if o.Value != "" && o.Name.Local != "" {
		req.Attr = append(req.Attr, xml.Attr(o))
	}
}

// WithSubtreeFilter sets the subtree `filter` to the `<get>` or `<get-config>` operation.
func WithSubtreeFilter(subtree string) GetOption { return filter(subtree) }

// WithDefaultMode sets the `with-defaults` in the `<get>` or `<get-config>` operation.
// This defines the behavior if default configs should be returned.
// See [DefaultsMode] for the available options.
func WithDefaultMode(op DefaultsMode) GetOption { return defaultsMode(op) }

// WithAttribute sets `attributes` in the `<get>` operation.
// For example juniper allows <get [format="(json | set | text | xml)"]> attributes in GetRequest.
func WithAttribute(key, value string) GetOption {
	return attr(xml.Attr{Name: xml.Name{Local: key}, Value: value})
}

type GetOption interface {
	apply(*GetRequest)
}

// GetConfig implements the <get-config> rpc operation defined in [RFC6241 7.1].
// `source` is the datastore to query.
//
// [RFC6241 7.1]: https://www.rfc-editor.org/rfc/rfc6241.html#section-7.1
func (s *Session) GetConfig(ctx context.Context, source Datastore, opts ...GetOption) (*RpcReply, error) {
	req := NewGetConfigRequest(source, opts...)
	if err := req.validate(s.serverCaps); err != nil {
		return nil, err
	}
	return s.do(ctx, req)
}

// Get issues the `<get>` operation as defined in [RFC6241 7.7]
// for retrieving running configuration and device state information.
//
// Only the `subtree` filter type is supported.
//
// [RFC6241 7.7] https://www.rfc-editor.org/rfc/rfc6241.html#section-7.7
func (s *Session) Get(ctx context.Context, opts ...GetOption) (*RpcReply, error) {
	req := NewGetRequest(opts...)
	if err := req.validate(s.serverCaps); err != nil {
		return nil, err
	}

	return s.do(ctx, req)
}
