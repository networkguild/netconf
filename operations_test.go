package netconf

import (
	"encoding/xml"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUnmarshalOk(t *testing.T) {
	tt := []struct {
		name  string
		input string
		want  bool
	}{
		{"selfclosing", "<foo>><ok/></foo>", true},
		{"missing", "<foo></foo>", false},
		{"closetag", "<foo><ok></ok></foo>", true},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			var v struct {
				XMLName xml.Name   `xml:"foo"`
				Ok      ExtantBool `xml:"ok"`
			}

			err := xml.Unmarshal([]byte(tc.input), &v)
			require.NoError(t, err)
			require.Equal(t, tc.want, bool(v.Ok))
		})
	}
}

func TestMarshalDatastore(t *testing.T) {
	tt := []struct {
		input     Datastore
		want      string
		shouldErr bool
	}{
		{Running, "<rpc><target><running/></target></rpc>", false},
		{Startup, "<rpc><target><startup/></target></rpc>", false},
		{Candidate, "<rpc><target><candidate/></target></rpc>", false},
		{Datastore{Store: "custom-store"}, "<rpc><target><custom-store/></target></rpc>", false},
		{Datastore{Store: ""}, "", true},
		{Datastore{Store: "<xml-elements>"}, "<rpc><target><&lt;xml-elements&gt;/></target></rpc>", true},
		{Datastore{Store: "candidate", Region: "bof"}, "<rpc><target><configuration-region>bof</configuration-region><candidate/></target></rpc>", false},
	}

	for _, tc := range tt {
		t.Run("test"+tc.input.Store, func(t *testing.T) {
			v := struct {
				XMLName xml.Name  `xml:"rpc"`
				Target  Datastore `xml:"target"`
			}{Target: tc.input}

			got, err := xml.Marshal(&v)
			if !tc.shouldErr {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, string(got))
		})
	}
}

type structuredCfg struct {
	XMLName xml.Name            `xml:"urn:ietf:params:xml:ns:netconf:base:1.0 config"`
	System  structuredCfgSystem `xml:"system"`
}

type structuredCfgSystem struct {
	Hostname string `xml:"host-name"`
}

const intfaceConfig = `
<interfaces>
  <interface>
    <name>ge-0/0/2</name>
    <unit>
      <name>0</name>
      <family>
        <inet>
          <address>
            <name>2.2.2.1/32</name>
          </address>
        </inet>
      </family>
    </unit>
  </interface>
</interfaces>
`

func TestEditConfig(t *testing.T) {
	tt := []struct {
		name      string
		target    Datastore
		config    any
		options   []EditConfigOption
		mustMatch []*regexp.Regexp
		noMatch   []*regexp.Regexp
	}{
		{
			name:   "running structured no options",
			target: Running,
			config: structuredCfg{
				System: structuredCfgSystem{
					Hostname: "darkstar",
				},
			},
			mustMatch: []*regexp.Regexp{
				regexp.MustCompile(`<target>\S*<running/>\S*</target>`),
				regexp.MustCompile(
					`<config xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">\S*<system>\S*<host-name>darkstar</host-name>\S*</system>\S*</config>`,
				),
			},
			noMatch: []*regexp.Regexp{
				regexp.MustCompile(`<url>`),
			},
		},
		{
			name:   "canidate string all options",
			target: Candidate,
			config: intfaceConfig,
			options: []EditConfigOption{
				WithDefaultMergeStrategy(ReplaceConfig),
				WithErrorStrategy(ContinueOnError),
				WithTestStrategy(TestOnly),
			},
			mustMatch: []*regexp.Regexp{
				regexp.MustCompile(`<target>\S*<candidate/>\S*</target>`),
				regexp.MustCompile(`<name>ge-0/0/2</name>`),
				regexp.MustCompile(`<default-operation>replace</default-operation>`),
				regexp.MustCompile(`<test-option>test-only</test-option>`),
				regexp.MustCompile(`<error-option>continue-on-error</error-option>`),
			},
			noMatch: []*regexp.Regexp{
				regexp.MustCompile(`<url>`),
			},
		},
		{
			name:   "byteslice config",
			target: Running,
			config: []byte("<system><services><ssh/></services></system>"),
			mustMatch: []*regexp.Regexp{
				regexp.MustCompile(`<system><services><ssh/></services></system>`),
			},
		},
		{
			name:   "startup url no options",
			target: Startup,
			config: URL("ftp://myftpesrver/foo/config.xml"),
			mustMatch: []*regexp.Regexp{
				regexp.MustCompile(`<target>\S*<startup/>\S*</target>`),
				regexp.MustCompile(`<url>ftp://myftpesrver/foo/config.xml</url>`),
			},
			noMatch: []*regexp.Regexp{
				regexp.MustCompile(`<config>`),
			},
		},
		{
			name: "candidate with bof configuration region no options",
			target: Datastore{
				Store:  "candidate",
				Region: "bof",
			},
			config: structuredCfg{
				System: structuredCfgSystem{
					Hostname: "bof",
				},
			},
			mustMatch: []*regexp.Regexp{
				regexp.MustCompile(`<target>\S*<configuration-region>bof</configuration-region><candidate/>\S*</target>`),
				regexp.MustCompile(
					`<config xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">\S*<system>\S*<host-name>bof</host-name>\S*</system>\S*</config>`,
				),
			},
			noMatch: []*regexp.Regexp{
				regexp.MustCompile(`<url>`),
			},
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTestServer(t)
			sess, _ := newSession(WithTransport(ts.transport()))
			go sess.recv()

			ts.queueRespString(`<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><ok/></rpc-reply>`)

			err := sess.EditConfig(t.Context(), tc.target, tc.config, tc.options...)
			require.NoError(t, err)

			sentMsg, err := ts.popReq()
			require.NoError(t, err)

			for _, match := range tc.mustMatch {
				require.Regexp(t, match, string(sentMsg))
			}

			for _, match := range tc.noMatch {
				require.NotRegexp(t, match, string(sentMsg))
			}
		})
	}
}

func TestDiscardChanges(t *testing.T) {
	tt := []struct {
		name   string
		source any
		match  string
	}{
		{
			name:  "discard-changes",
			match: `<rpc xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><discard-changes xmlns="urn:ietf:params:xml:ns:netconf:base:1.0"></discard-changes></rpc>`,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTestServer(t)
			sess, _ := newSession(WithTransport(ts.transport()))
			sess.serverCaps = newCapabilitySet(CandidateCapability)
			go sess.recv()

			ts.queueRespString(`<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><ok/></rpc-reply>`)

			err := sess.DiscardChanges(t.Context())
			require.NoError(t, err)

			sentMsg, err := ts.popReq()
			require.NoError(t, err)

			require.Equal(t, tc.match, string(sentMsg))
		})
	}
}

func TestCopyConfig(t *testing.T) {
	tt := []struct {
		name           string
		source, target any
		matches        []*regexp.Regexp
	}{
		{
			name:   "running->startup",
			source: Running,
			target: Startup,
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<source>\S*<running/>\S*</source>`),
				regexp.MustCompile(`<target>\S*<startup/>\S*</target>`),
			},
		},
		{
			name:   "running->url",
			source: Running,
			target: URL("ftp://myserver.example.com/router.cfg"),
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<source>\S*<running/>\S*</source>`),
				regexp.MustCompile(`<target>\S*<url>ftp://myserver.example.com/router.cfg</url>\S*</target>`),
			},
		},
		{
			name:   "url->candidate",
			source: URL("http://myserver.example.com/router.cfg"),
			target: Candidate,
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<source>\S*<url>http://myserver.example.com/router.cfg</url>\S*</source>`),
				regexp.MustCompile(`<target>\S*<candidate/>\S*</target>`),
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTestServer(t)
			sess, _ := newSession(WithTransport(ts.transport()))
			go sess.recv()

			ts.queueRespString(`<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><ok/></rpc-reply>`)

			err := sess.CopyConfig(t.Context(), tc.source, tc.target)
			require.NoError(t, err)

			sentMsg, err := ts.popReq()
			require.NoError(t, err)

			for _, match := range tc.matches {
				require.Regexp(t, match, string(sentMsg))
			}
		})
	}
}

func TestDeleteConfig(t *testing.T) {
	tt := []struct {
		target  Datastore
		matches []*regexp.Regexp
	}{
		{
			target: Startup,
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<delete-config xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">\S*<target>\S*<startup/>\S*</target>\S*</delete-config>`),
			},
		},
	}

	for _, tc := range tt {
		t.Run("test"+tc.target.Store, func(t *testing.T) {
			ts := newTestServer(t)
			sess, _ := newSession(WithTransport(ts.transport()))
			go sess.recv()

			ts.queueRespString(`<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><ok/></rpc-reply>`)

			err := sess.DeleteConfig(t.Context(), tc.target)
			require.NoError(t, err)

			sentMsg, err := ts.popReq()
			require.NoError(t, err)

			for _, match := range tc.matches {
				require.Regexp(t, match, string(sentMsg))
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	tt := []struct {
		name    string
		source  any
		matches []*regexp.Regexp
	}{
		{
			name:   "candidate",
			source: Candidate,
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<validate xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">\S*<source>\S*<candidate/>\S*</source>\S*</validate>`),
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTestServer(t)
			sess, _ := newSession(WithTransport(ts.transport()))
			sess.serverCaps = newCapabilitySet(ValidateCapability)
			go sess.recv()

			ts.queueRespString(`<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><ok/></rpc-reply>`)

			err := sess.Validate(t.Context(), tc.source)
			require.NoError(t, err)

			sentMsg, err := ts.popReq()
			require.NoError(t, err)

			for _, match := range tc.matches {
				require.Regexp(t, match, string(sentMsg))
			}
		})
	}
}

func TestLock(t *testing.T) {
	tt := []struct {
		target  Datastore
		matches []*regexp.Regexp
	}{
		{
			target: Candidate,
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<lock xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">\S*<target>\S*<candidate/>\S*</target>\S*</lock>`),
			},
		},
	}

	for _, tc := range tt {
		t.Run("test"+tc.target.Store, func(t *testing.T) {
			ts := newTestServer(t)
			sess, _ := newSession(WithTransport(ts.transport()))
			go sess.recv()

			ts.queueRespString(`<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><ok/></rpc-reply>`)

			err := sess.Lock(t.Context(), tc.target)
			require.NoError(t, err)

			sentMsg, err := ts.popReq()
			require.NoError(t, err)

			for _, match := range tc.matches {
				require.Regexp(t, match, string(sentMsg))
			}
		})
	}
}

func TestUnlock(t *testing.T) {
	tt := []struct {
		target  Datastore
		matches []*regexp.Regexp
	}{
		{
			target: Candidate,
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<unlock xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">\S*<target>\S*<candidate/>\S*</target>\S*</unlock>`),
			},
		},
	}

	for _, tc := range tt {
		t.Run("test"+tc.target.Store, func(t *testing.T) {
			ts := newTestServer(t)
			sess, _ := newSession(WithTransport(ts.transport()))
			go sess.recv()

			ts.queueRespString(`<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><ok/></rpc-reply>`)

			err := sess.Unlock(t.Context(), tc.target)
			require.NoError(t, err)

			sentMsg, err := ts.popReq()
			require.NoError(t, err)

			for _, match := range tc.matches {
				require.Regexp(t, match, string(sentMsg))
			}
		})
	}
}

func TestKillSession(t *testing.T) {
	tt := []struct {
		id      uint64
		matches []*regexp.Regexp
	}{
		{
			id: 42,
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<kill-session xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">\S*<session-id>42</session-id>\S*</kill-session>`),
			},
		},
	}

	for _, tc := range tt {
		t.Run(strconv.Itoa(int(tc.id)), func(t *testing.T) {
			ts := newTestServer(t)
			sess, _ := newSession(WithTransport(ts.transport()))
			go sess.recv()

			ts.queueRespString(`<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><ok/></rpc-reply>`)

			reply, err := sess.KillSession(t.Context(), tc.id)
			require.NoError(t, err)
			require.NotNil(t, reply)

			sentMsg, err := ts.popReq()
			require.NoError(t, err)

			for _, match := range tc.matches {
				require.Regexp(t, match, string(sentMsg))
			}
		})
	}
}

func TestCommit(t *testing.T) {
	tt := []struct {
		name    string
		options []CommitOption
		matches []*regexp.Regexp
	}{
		{
			name: "noOptions",
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<commit xmlns="urn:ietf:params:xml:ns:netconf:base:1.0"></commit>`),
			},
		},
		{
			name:    "confirmed",
			options: []CommitOption{WithConfirmed()},
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<commit xmlns="urn:ietf:params:xml:ns:netconf:base:1.0"><confirmed></confirmed></commit>`),
			},
		},
		{
			name:    "confirmed",
			options: []CommitOption{WithConfirmedTimeout(1 * time.Minute)},
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<commit xmlns="urn:ietf:params:xml:ns:netconf:base:1.0"><confirmed></confirmed><confirm-timeout>60</confirm-timeout></commit>`),
			},
		},
		{
			name:    "persist",
			options: []CommitOption{WithPersist("myid")},
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<commit xmlns="urn:ietf:params:xml:ns:netconf:base:1.0"><confirmed></confirmed><persist>myid</persist></commit>`),
			},
		},
		{
			name:    "persist_id",
			options: []CommitOption{WithPersistID("myid")},
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<commit xmlns="urn:ietf:params:xml:ns:netconf:base:1.0"><persist-id>myid</persist-id></commit>`),
			},
		},
		{
			name:    "configuration region",
			options: []CommitOption{WithConfigurationRegion("bof")},
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<commit xmlns="urn:ietf:params:xml:ns:netconf:base:1.0"><configuration-region>bof</configuration-region></commit>`),
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTestServer(t)
			sess, _ := newSession(WithTransport(ts.transport()))
			sess.serverCaps = newCapabilitySet(ConfirmedCommitCapability)
			go sess.recv()

			ts.queueRespString(`<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><ok/></rpc-reply>`)

			err := sess.Commit(t.Context(), tc.options...)
			require.NoError(t, err)

			sentMsg, err := ts.popReq()
			require.NoError(t, err)

			for _, match := range tc.matches {
				require.Regexp(t, match, string(sentMsg))
			}
		})
	}
}

func TestCancelCommit(t *testing.T) {
	tt := []struct {
		name    string
		options []CancelCommitOption
		matches []*regexp.Regexp
	}{
		{
			name: "noOptions",
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<cancel-commit xmlns="urn:ietf:params:xml:ns:netconf:base:1.0"></cancel-commit>`),
			},
		},
		{
			name:    "persist_id",
			options: []CancelCommitOption{WithPersistID("myid")},
			matches: []*regexp.Regexp{
				regexp.MustCompile(`<cancel-commit xmlns="urn:ietf:params:xml:ns:netconf:base:1.0"><persist-id>myid</persist-id></cancel-commit>`),
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			ts := newTestServer(t)
			sess, _ := newSession(WithTransport(ts.transport()))
			go sess.recv()

			ts.queueRespString(`<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><ok/></rpc-reply>`)

			err := sess.CancelCommit(t.Context(), tc.options...)
			require.NoError(t, err)

			sentMsg, err := ts.popReq()
			require.NoError(t, err)

			for _, match := range tc.matches {
				require.Regexp(t, match, string(sentMsg))
			}
		})
	}
}
