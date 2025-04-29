package netconf

import (
	"encoding/xml"
	"testing"

	"github.com/stretchr/testify/require"
)

var rawXMLTests = []struct {
	name        string
	element     RawXML
	xml         []byte
	noUnmarshal bool
}{
	{
		name:    "empty",
		element: RawXML(""),
		xml:     []byte("<RawXML></RawXML>"),
	},
	{
		name:        "nil",
		element:     nil,
		xml:         []byte("<RawXML></RawXML>"),
		noUnmarshal: true,
	},
	{
		name:    "textElement",
		element: RawXML("A man a plan a canal panama"),
		xml:     []byte("<RawXML>A man a plan a canal panama</RawXML>"),
	},
	{
		name:    "xml",
		element: RawXML("<foo><bar>hamburger</bar></foo>"),
		xml:     []byte("<RawXML><foo><bar>hamburger</bar></foo></RawXML>"),
	},
}

func TestRawXMLUnmarshal(t *testing.T) {
	for _, tc := range rawXMLTests {
		if tc.noUnmarshal {
			continue
		}

		t.Run(tc.name, func(t *testing.T) {
			var got RawXML
			err := xml.Unmarshal(tc.xml, &got)
			require.NoError(t, err)
			require.Equal(t, tc.element, got)
		})
	}
}

func TestRawXMLMarshal(t *testing.T) {
	for _, tc := range rawXMLTests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := xml.Marshal(&tc.element)
			require.NoError(t, err)
			require.Equal(t, tc.xml, got)
		})
	}
}

var helloMsgTestTable = []struct {
	name string
	raw  []byte
	msg  Hello
}{
	{
		name: "basic",
		raw:  []byte(`<hello xmlns="urn:ietf:params:xml:ns:netconf:base:1.0"><capabilities><capability>urn:ietf:params:netconf:base:1.0</capability><capability>urn:ietf:params:netconf:base:1.1</capability></capabilities></hello>`),
		msg: Hello{
			XMLName: xml.Name{
				Local: "hello",
				Space: "urn:ietf:params:xml:ns:netconf:base:1.0",
			},
			Capabilities: []string{
				"urn:ietf:params:netconf:base:1.0",
				"urn:ietf:params:netconf:base:1.1",
			},
		},
	},
	{
		name: "junos",
		raw: []byte(`<hello xmlns="urn:ietf:params:xml:ns:netconf:base:1.0">
  <capabilities>
      <capability>urn:ietf:params:netconf:base:1.0</capability>
	  <capability>urn:ietf:params:netconf:capability:candidate:1.0</capability>
	  <capability>urn:ietf:params:netconf:capability:confirmed-commit:1.0</capability>
	  <capability>urn:ietf:params:netconf:capability:validate:1.0</capability>
	  <capability>urn:ietf:params:netconf:capability:url:1.0?scheme=http,ftp,file</capability>
	  <capability>urn:ietf:params:xml:ns:netconf:base:1.0</capability>
	  <capability>urn:ietf:params:xml:ns:netconf:capability:candidate:1.0</capability>
	  <capability>urn:ietf:params:xml:ns:netconf:capability:confirmed-commit:1.0</capability>
	  <capability>urn:ietf:params:xml:ns:netconf:capability:validate:1.0</capability>
	  <capability>urn:ietf:params:xml:ns:netconf:capability:url:1.0?scheme=http,ftp,file</capability>
	  <capability>urn:ietf:params:xml:ns:yang:ietf-netconf-monitoring</capability>
	  <capability>http://xml.juniper.net/netconf/jdm/1.0</capability>
  </capabilities>
  <session-id>410</session-id>
</hello>`),
		msg: Hello{
			XMLName: xml.Name{
				Local: "hello",
				Space: "urn:ietf:params:xml:ns:netconf:base:1.0",
			},
			Capabilities: []string{
				"urn:ietf:params:netconf:base:1.0",
				"urn:ietf:params:netconf:capability:candidate:1.0",
				"urn:ietf:params:netconf:capability:confirmed-commit:1.0",
				"urn:ietf:params:netconf:capability:validate:1.0",
				"urn:ietf:params:netconf:capability:url:1.0?scheme=http,ftp,file",
				"urn:ietf:params:xml:ns:netconf:base:1.0",
				"urn:ietf:params:xml:ns:netconf:capability:candidate:1.0",
				"urn:ietf:params:xml:ns:netconf:capability:confirmed-commit:1.0",
				"urn:ietf:params:xml:ns:netconf:capability:validate:1.0",
				"urn:ietf:params:xml:ns:netconf:capability:url:1.0?scheme=http,ftp,file",
				"urn:ietf:params:xml:ns:yang:ietf-netconf-monitoring",
				"http://xml.juniper.net/netconf/jdm/1.0",
			},
			SessionID: 410,
		},
	},
}

func TestUnmarshalHelloMsg(t *testing.T) {
	for _, tc := range helloMsgTestTable {
		t.Run(tc.name, func(t *testing.T) {
			var got Hello
			err := xml.Unmarshal(tc.raw, &got)
			require.NoError(t, err)
			require.Equal(t, got, tc.msg)
		})
	}
}

func TestMarshalHelloMsg(t *testing.T) {
	for _, tc := range helloMsgTestTable {
		t.Run(tc.name, func(t *testing.T) {
			_, err := xml.Marshal(tc.msg)
			require.NoError(t, err)
		})
	}
}

func TestMarshalRPCMsg(t *testing.T) {
	tt := []struct {
		name      string
		operation any
		err       bool
		want      []byte
	}{
		{
			name:      "nil",
			operation: nil,
			err:       true,
		},
		{
			name:      "string",
			operation: "<foo><bar/></foo>",
			want:      []byte(`<rpc xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><foo><bar/></foo></rpc>`),
		},
		{
			name:      "byteslice",
			operation: []byte("<baz><qux/></baz>"),
			want:      []byte(`<rpc xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><baz><qux/></baz></rpc>`),
		},
		{
			name:      "validate",
			operation: ValidateRequest{Source: Running},
			want:      []byte(`<rpc xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><validate xmlns="urn:ietf:params:xml:ns:netconf:base:1.0"><source><running/></source></validate></rpc>`),
		},
		{
			name: "namedStruct",
			operation: struct {
				XMLName xml.Name `xml:"http://xml.juniper.net/junos/22.4R0/junos command"`
				Command string   `xml:",innerxml"`
			}{
				Command: "show bgp neighbors",
			},
			want: []byte(`<rpc xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1"><command xmlns="http://xml.juniper.net/junos/22.4R0/junos">show bgp neighbors</command></rpc>`),
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			out, err := xml.Marshal(&Rpc{
				MessageID: 1,
				Operation: tc.operation,
			})

			if tc.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, out, tc.want)
			}
		})
	}
}

var replyJunosGetConfigError = []byte(`
<rpc-reply xmlns:junos="http://xml.juniper.net/junos/20.3R0/junos" xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="1">
<rpc-error>
<error-type>protocol</error-type>
<error-tag>operation-failed</error-tag>
<error-severity>error</error-severity>
<error-message>syntax error, expecting &lt;candidate/&gt; or &lt;running/&gt;</error-message>
<error-info>
<bad-element>non-exist</bad-element>
</error-info>
</rpc-error>
</rpc-reply>
`)

func TestUnmarshalRPCReply(t *testing.T) {
	tt := []struct {
		name  string
		reply []byte
		want  RpcReply
	}{
		{
			name:  "error",
			reply: replyJunosGetConfigError,
			want: RpcReply{
				XMLName: xml.Name{
					Local: "rpc-reply",
					Space: "urn:ietf:params:xml:ns:netconf:base:1.0",
				},
				MessageID: 1,
				Errors: []RPCError{
					{
						Type:     ErrTypeProtocol,
						Tag:      ErrOperationFailed,
						Severity: SevError,
						Message:  "syntax error, expecting <candidate/> or <running/>",
						Info: []byte(`
<bad-element>non-exist</bad-element>
`),
					},
				},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			var got RpcReply
			err := xml.Unmarshal(tc.reply, &got)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
