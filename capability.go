package netconf

import (
	"maps"
	"net/url"
	"slices"
	"strings"
)

const (
	BaseCapability = "urn:ietf:params:netconf:base"
	stdCapPrefix   = "urn:ietf:params:netconf:capability"
	baseNetconfNs  = "urn:ietf:params:xml:ns:netconf:base:1.0"
)

const (
	WritableRunningCapability     = "urn:ietf:params:netconf:capability:writable-running:1.0"
	StartupCapability             = "urn:ietf:params:netconf:capability:startup:1.0"
	CandidateCapability           = "urn:ietf:params:netconf:capability:candidate:1.0"
	RollbackOnErrorCapability     = "urn:ietf:params:netconf:capability:rollback-on-error:1.0"
	URLCapability                 = "urn:ietf:params:netconf:capability:url:1.0"
	XPathCapability               = "urn:ietf:params:netconf:capability:xpath:1.0"
	ConfirmedCommitCapability     = "urn:ietf:params:netconf:capability:confirmed-commit:1.1"
	ValidateOldCapability         = "urn:ietf:params:netconf:capability:validate:1.0"
	ValidateCapability            = "urn:ietf:params:netconf:capability:validate:1.1"
	WithDefaultsCapability        = "urn:ietf:params:netconf:capability:with-defaults:1.0"
	NotificationCapability        = "urn:ietf:params:netconf:capability:notification:1.0"
	DynamicNotificationCapability = "urn:ietf:params:netconf:capability:notification:2.0"
	InterleaveCapability          = "urn:ietf:params:netconf:capability:interleave:1.0"
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

type capabilitySet map[string]url.Values

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
		capability, values := parseQuery(ExpandCapability(c))
		cs[capability] = values
	}
}

func (cs capabilitySet) Has(cap string) bool {
	if len(cs) == 0 {
		return false
	}
	_, ok := cs[ExpandCapability(cap)]
	return ok
}

func (cs capabilitySet) ContainsValue(cap, key, value string) bool {
	if len(cs) == 0 {
		return false
	}

	values, ok := cs[ExpandCapability(cap)]
	if !ok {
		return false
	}

	valuesSlice, ok := values[key]
	if !ok {
		return false
	}

	return slices.ContainsFunc(valuesSlice, func(v string) bool {
		return slices.Contains(strings.Split(v, ","), value)
	})
}

func (cs capabilitySet) All() []string {
	if len(cs) == 0 {
		return []string{}
	}
	return slices.Collect(maps.Keys(cs))
}

func parseQuery(s string) (string, url.Values) {
	if strings.Contains(s, "?") {
		parts := strings.Split(s, "?")
		if len(parts) > 1 {
			values, _ := url.ParseQuery(parts[1])
			return parts[0], values
		}
	}
	return s, nil
}
