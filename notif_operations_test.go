package netconf

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCreateSubscription(t *testing.T) {
	start := time.Date(2023, time.June, 07, 18, 31, 48, 00, time.UTC)
	end := time.Date(2023, time.June, 07, 18, 33, 48, 00, time.UTC)

	tt := []struct {
		name    string
		options []CreateSubscriptionOption
		matches []*regexp.Regexp
	}{
		{
			name: "noOptions",
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<create-subscription xmlns="urn:ietf:params:xml:ns:netconf:notification:1.0"></create-subscription>`),
			},
		},
		{
			name:    "startTime option",
			options: []CreateSubscriptionOption{WithStartTimeOption(start)},
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<create-subscription xmlns="urn:ietf:params:xml:ns:netconf:notification:1.0"><startTime>` + regexp.QuoteMeta(start.Format(time.RFC3339)) + `</startTime></create-subscription>`),
			},
		},
		{
			name:    "endTime option",
			options: []CreateSubscriptionOption{WithStopTimeOption(end)},
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<create-subscription xmlns="urn:ietf:params:xml:ns:netconf:notification:1.0"><stopTime>` + regexp.QuoteMeta(end.Format(time.RFC3339)) + `</stopTime></create-subscription>`),
			},
		},
		{
			name:    "stream option",
			options: []CreateSubscriptionOption{WithStreamOption("thestream")},
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<create-subscription xmlns="urn:ietf:params:xml:ns:netconf:notification:1.0"><stream>thestream</stream></create-subscription>`),
			},
		},
		{
			name: "stream option and filter",
			options: []CreateSubscriptionOption{
				WithStreamOption("NETCONF"),
				WithFilterOption(`<netconf-config-change xmlns="urn:ietf:params:xml:ns:yang:ietf-netconf-notifications"/>`),
			},
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<create-subscription xmlns="urn:ietf:params:xml:ns:netconf:notification:1.0"><stream>NETCONF</stream><filter type="subtree"><netconf-config-change xmlns="urn:ietf:params:xml:ns:yang:ietf-netconf-notifications"/></filter></create-subscription>`),
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTestServer(t)
			sess := newSession(ts.transport())
			sess.serverCaps = newCapabilitySet(NotificationCapability)
			go sess.recv()

			ts.queueRespString(`<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><ok/></rpc-reply>`)

			err := sess.CreateSubscription(context.Background(), tc.options...)
			assert.NoError(t, err)

			sentMsg, err := ts.popReq()
			assert.NoError(t, err)

			for _, match := range tc.matches {
				assert.Regexp(t, match, string(sentMsg))
			}
		})
	}
}
