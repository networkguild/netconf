package netconf

const (
	baseCap       = "urn:ietf:params:netconf:base"
	stdCapPrefix  = "urn:ietf:params:netconf:capability"
	baseNetconfNs = "urn:ietf:params:xml:ns:netconf:base:1.0"
)

const (
	WritableRunningCapability = "urn:ietf:params:netconf:capability:writable-running:1.0"
	StartupCapability         = "urn:ietf:params:netconf:capability:startup:1.0"
	CandidateCapability       = "urn:ietf:params:netconf:capability:candidate:1.0"
	RollbackOnErrorCapability = "urn:ietf:params:netconf:capability:rollback-on-error:1.0"
	URLCapability             = "urn:ietf:params:netconf:capability:url:1.0"
	ConfirmedCommitCapability = "urn:ietf:params:netconf:capability:confirmed-commit:1.1"
	ValidateCapability        = "urn:ietf:params:netconf:capability:validate:1.1"
	NotificationCapability    = "urn:ietf:params:netconf:capability:notification:1.0"
)

// DefaultCapabilities are the capabilities sent by the client during the hello
// exchange by the server.
var DefaultCapabilities = []string{
	"urn:ietf:params:netconf:base:1.0",
	"urn:ietf:params:netconf:base:1.1",
}

// ExpandCapability will automatically add the standard capability prefix of
// `urn:ietf:params:netconf:capability` if not already present.
func ExpandCapability(s string) string {
	if s == "" {
		return ""
	}

	if s[0] != ':' {
		return s
	}

	return stdCapPrefix + s
}

// XXX: may want to expose this type publicly in the future when the api has
// stabilized?
type capabilitySet struct {
	caps map[string]struct{}
}

func newCapabilitySet(capabilities ...string) capabilitySet {
	cs := capabilitySet{
		caps: make(map[string]struct{}),
	}
	cs.Add(capabilities...)
	return cs
}

func (cs *capabilitySet) Add(capabilities ...string) {
	for _, cap := range capabilities {
		cap = ExpandCapability(cap)
		cs.caps[cap] = struct{}{}
	}
}

func (cs capabilitySet) Has(s string) bool {
	s = ExpandCapability(s)
	_, ok := cs.caps[s]
	return ok
}

func (cs capabilitySet) All() []string {
	out := make([]string, 0, len(cs.caps))
	for c := range cs.caps {
		out = append(out, c)
	}
	return out
}
