package netconf

import (
	"context"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetConfig(t *testing.T) {
	tt := []struct {
		name    string
		source  Datastore
		options []GetOption
		matches []*regexp.Regexp
	}{
		{
			name:   "get-config startup with-defaults",
			source: Startup,
			options: []GetOption{
				WithDefaultMode(DefaultsModeTrim),
			},
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<source>\S*<startup/>\S*</source>`),
				regexp.MustCompile(`<with-defaults xmlns="urn:ietf:params:xml:ns:yang:ietf-netconf-with-defaults">trim</with-defaults>`),
			},
		},
		{
			name:   "get-config running filter",
			source: Running,
			options: []GetOption{
				WithFilter(`<interfaces xmlns="urn:ietf:params:xml:ns:yang:ietf-interfaces"/>`),
			},
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<source>\S*<running/>\S*</source>`),
				regexp.MustCompile(`<filter type="subtree"><interfaces xmlns="urn:ietf:params:xml:ns:yang:ietf-interfaces"/></filter>`),
			},
		},
		{
			name:   "get-config running no options",
			source: Running,
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<get-config xmlns="urn:ietf:params:xml:ns:netconf:base:1.0"><source><running/></source></get-config>`),
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTestServer(t)
			sess := newSession(ts.transport())
			go sess.recv()

			ts.queueRespString(`<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><ok/></rpc-reply>`)

			reply, err := sess.GetConfig(context.Background(), tc.source, tc.options...)
			assert.NoError(t, err)
			assert.NotNil(t, reply)

			sentMsg, err := ts.popReq()
			assert.NoError(t, err)

			for _, match := range tc.matches {
				assert.Regexp(t, match, string(sentMsg))
			}
		})
	}
}

func TestGet(t *testing.T) {
	tt := []struct {
		name    string
		options []GetOption
		matches []*regexp.Regexp
	}{
		{
			name: "get ifm",
			options: []GetOption{
				WithFilter(`<ifm xmlns="urn:huawei:yang:huawei-ifm"/>`),
			},
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<get xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">\S*<filter type="subtree">\S*<ifm xmlns="urn:huawei:yang:huawei-ifm"/>\S*</filter>\S*</get>`),
			},
		},
		{
			name: "get devm",
			options: []GetOption{
				WithDefaultMode("report-all"),
				WithFilter(`<devm xmlns="urn:huawei:yang:huawei-devm"/>`),
			},
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<get xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">\S*<filter type="subtree">\S*<devm xmlns="urn:huawei:yang:huawei-devm"/>\S*</filter>\S*<with-defaults xmlns="urn:ietf:params:xml:ns:yang:ietf-netconf-with-defaults">report-all</with-defaults>\S*</get>`),
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTestServer(t)
			sess := newSession(ts.transport())
			go sess.recv()

			ts.queueRespString(`<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><data>daa</data></rpc-reply>`)

			reply, err := sess.Get(context.Background(), tc.options...)
			assert.NoError(t, err)
			assert.NotNil(t, reply)

			sentMsg, err := ts.popReq()
			assert.NoError(t, err)

			for _, match := range tc.matches {
				assert.Regexp(t, match, string(sentMsg))
			}
		})
	}
}
