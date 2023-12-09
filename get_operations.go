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

type GetRequest struct {
	XMLName      xml.Name
	Source       Datastore    `xml:"source,omitempty"`
	Filter       Filter       `xml:"filter,omitempty"`
	WithDefaults DefaultsMode `xml:"urn:ietf:params:xml:ns:yang:ietf-netconf-with-defaults with-defaults,omitempty"`
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

func (o filter) apply(req *GetRequest)       { req.Filter = Filter(o) }
func (o defaultsMode) apply(req *GetRequest) { req.WithDefaults = DefaultsMode(o) }

// WithSubtreeFilter sets the subtree `filter` to the `<get>` or `<get-config>` operation.
func WithSubtreeFilter(subtree string) GetOption { return filter(subtree) }

// WithDefaultMode sets the `with-defaults` in the `<get>` or `<get-config>` operation.
// This defines the behavior if default configs should be returned.
// See [DefaultsMode] for the available options.
func WithDefaultMode(op DefaultsMode) GetOption { return defaultsMode(op) }

type GetOption interface {
	apply(*GetRequest)
}

// GetConfig implements the <get-config> rpc operation defined in [RFC6241 7.1].
// `source` is the datastore to query.
//
// [RFC6241 7.1]: https://www.rfc-editor.org/rfc/rfc6241.html#section-7.1
func (s *Session) GetConfig(ctx context.Context, source Datastore, opts ...GetOption) (*Reply, error) {
	req := GetRequest{
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
	req := GetRequest{
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
