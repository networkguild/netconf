package netconf

import (
	"context"
	"encoding/xml"
)

func (f Filter) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	v := struct {
		Type   string `xml:"type,attr,omitempty"`
		Filter string `xml:",innerxml"`
	}{Type: "subtree", Filter: string(f)}
	return e.EncodeElement(&v, start)
}

type GetReq struct {
	XMLName      xml.Name
	Source       Datastore    `xml:"source,omitempty"`
	Filter       Filter       `xml:"filter,omitempty"`
	WithDefaults DefaultsMode `xml:"urn:ietf:params:xml:ns:yang:ietf-netconf-with-defaults with-defaults,omitempty"`
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

func (o filter) apply(req *GetReq)       { req.Filter = Filter(o) }
func (o defaultsMode) apply(req *GetReq) { req.WithDefaults = DefaultsMode(o) }

// WithFilter sets the `filter` to the `<get>` or `<get-config>` operation.
// Filter is vendor specific subtree filter.
func WithFilter(f Filter) GetOption { return filter(f) }

// WithDefaultMode sets the `with-defaults` in the `<get>` or `<get-config>` operation.
// This defines the behavior if default configs should be returned.
// See [DefaultsMode] for the available options.
func WithDefaultMode(op DefaultsMode) GetOption { return defaultsMode(op) }

type GetOption interface {
	apply(*GetReq)
}

// GetConfig implements the <get-config> rpc operation defined in [RFC6241 7.1].
// `source` is the datastore to query.
//
// [RFC6241 7.1]: https://www.rfc-editor.org/rfc/rfc6241.html#section-7.1
func (s *Session) GetConfig(ctx context.Context, source Datastore, opts ...GetOption) (*Reply, error) {
	req := GetReq{
		XMLName: xml.Name{Space: baseNetconfNs, Local: "get-config"},
		Source:  source,
	}

	for _, opt := range opts {
		opt.apply(&req)
	}

	return s.Do(ctx, &req)
}

// Get issues the `<get>` operation as defined in [RFC6241 7.7]
// for retrieving running configuration and device state information.
//
// Only the `subtree` filter type is supported.
//
// [RFC6241 7.7] https://www.rfc-editor.org/rfc/rfc6241.html#section-7.7
func (s *Session) Get(ctx context.Context, opts ...GetOption) (*Reply, error) {
	req := GetReq{
		XMLName: xml.Name{Space: baseNetconfNs, Local: "get"},
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
