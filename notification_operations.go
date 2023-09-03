package netconf

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"
)

// CreateSubscriptionOption is a optional arguments to [Session.CreateSubscription] method
type CreateSubscriptionOption interface {
	apply(req *CreateSubscriptionReq)
}

type CreateSubscriptionReq struct {
	XMLName   xml.Name `xml:"urn:ietf:params:xml:ns:netconf:notification:1.0 create-subscription"`
	Stream    string   `xml:"stream,omitempty"`
	Filter    Filter   `xml:"filter,omitempty"`
	StartTime string   `xml:"startTime,omitempty"`
	StopTime  string   `xml:"stopTime,omitempty"`
}

type stream string
type startTime time.Time
type stopTime time.Time

func (o stream) apply(req *CreateSubscriptionReq) {
	req.Stream = string(o)
}
func (o startTime) apply(req *CreateSubscriptionReq) {
	req.StartTime = time.Time(o).Format(time.RFC3339)
}
func (o stopTime) apply(req *CreateSubscriptionReq) {
	req.StopTime = time.Time(o).Format(time.RFC3339)
}

func WithStreamOption(s string) CreateSubscriptionOption        { return stream(s) }
func WithStartTimeOption(st time.Time) CreateSubscriptionOption { return startTime(st) }
func WithStopTimeOption(et time.Time) CreateSubscriptionOption  { return stopTime(et) }

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
