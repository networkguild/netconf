package netconf

import (
	"maps"
	"slices"
)

const (
	BaseCapability = "urn:ietf:params:netconf:base"
	stdCapPrefix   = "urn:ietf:params:netconf:capability"
	baseNetconfNs  = "urn:ietf:params:xml:ns:netconf:base:1.0"
)

const (
	WritableRunningCapability = "urn:ietf:params:netconf:capability:writable-running:1.0"
	StartupCapability         = "urn:ietf:params:netconf:capability:startup:1.0"
	CandidateCapability       = "urn:ietf:params:netconf:capability:candidate:1.0"
	RollbackOnErrorCapability = "urn:ietf:params:netconf:capability:rollback-on-error:1.0"
	URLCapability             = "urn:ietf:params:netconf:capability:url:1.0"
	ConfirmedCommitCapability = "urn:ietf:params:netconf:capability:confirmed-commit:1.1"
	ValidateOldCapability     = "urn:ietf:params:netconf:capability:validate:1.0"
	ValidateCapability        = "urn:ietf:params:netconf:capability:validate:1.1"
	NotificationCapability    = "urn:ietf:params:netconf:capability:notification:1.0"
)

// DefaultCapabilities are the capabilities sent by the client during the hello
// exchange by the server.
var DefaultCapabilities = []string{
	BaseCapability + ":1.0",
	BaseCapability + ":1.1",
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

type capabilitySet map[string]struct{}

func newCapabilitySet(capabilities ...string) capabilitySet {
	cs := make(capabilitySet, len(capabilities))
	cs.Add(capabilities...)
	return cs
}

func (cs capabilitySet) Add(capabilities ...string) {
	if cs == nil {
		return
	}
	for _, c := range capabilities {
		cs[ExpandCapability(c)] = struct{}{}
	}
}

func (cs capabilitySet) Has(s string) bool {
	if cs == nil {
		return false
	}
	_, ok := cs[ExpandCapability(s)]
	return ok
}

func (cs capabilitySet) All() []string {
	if len(cs) == 0 {
		return []string{}
	}
	return slices.Collect(maps.Keys(cs))
}
