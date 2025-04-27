package netconf

import (
	"context"
	"encoding/xml"
	"errors"
	"time"
)

// CreateSubscriptionOption is a optional arguments to [Session.CreateSubscription] method.
type CreateSubscriptionOption interface {
	apply(req *CreateSubscriptionRequest)
}

type CreateSubscriptionRequest struct {
	XMLName   xml.Name `xml:"urn:ietf:params:xml:ns:netconf:notification:1.0 create-subscription"`
	Stream    string   `xml:"stream,omitempty"`
	Filter    Filter   `xml:"filter,omitempty"`
	StartTime string   `xml:"startTime,omitempty"`
	StopTime  string   `xml:"stopTime,omitempty"`
}

func NewSubscriptionRequest(opts ...CreateSubscriptionOption) *CreateSubscriptionRequest {
	var req CreateSubscriptionRequest
	for _, opt := range opts {
		opt.apply(&req)
	}

	return &req
}

type stream string

type startTime time.Time

type stopTime time.Time

type subscriptionFilter Filter

func (o stream) apply(req *CreateSubscriptionRequest) {
	req.Stream = string(o)
}

func (o startTime) apply(req *CreateSubscriptionRequest) {
	req.StartTime = time.Time(o).Format(time.RFC3339)
}

func (o stopTime) apply(req *CreateSubscriptionRequest) {
	req.StopTime = time.Time(o).Format(time.RFC3339)
}

func (o subscriptionFilter) apply(req *CreateSubscriptionRequest) {
	req.Filter = Filter(o)
}

func WithStreamOption(s string) CreateSubscriptionOption        { return stream(s) }
func WithStartTimeOption(st time.Time) CreateSubscriptionOption { return startTime(st) }
func WithStopTimeOption(et time.Time) CreateSubscriptionOption  { return stopTime(et) }
func WithFilterOption(subtree string) CreateSubscriptionOption  { return subscriptionFilter(subtree) }

// CreateSubscription issues the `<create-subscription>` operation as defined in [RFC5277 2.1.1]
// for initiating an event notification subscription that will send asynchronous event notifications to the initiator.
//
// This requires the device to support the [NotificationCapability] capability
//
// [RFC5277 2.1.1] https://www.rfc-editor.org/rfc/rfc5277.html#section-2.1.1
func (s *Session) CreateSubscription(ctx context.Context, opts ...CreateSubscriptionOption) error {
	if !s.serverCaps.Has(NotificationCapability) {
		return errors.New("server does not support notifications")
	}

	if s.notificationHandler == nil {
		return errors.New("notification handler not set")
	}

	return s.call(ctx, NewSubscriptionRequest(opts...), nil)
}
