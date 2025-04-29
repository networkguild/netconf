package netconf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewCapabilitySet(t *testing.T) {
	caps := newCapabilitySet(DefaultCapabilities...)
	assert.True(t, caps.Has("urn:ietf:params:netconf:base:1.0"))
	assert.True(t, caps.Has("urn:ietf:params:netconf:base:1.1"))
}

func TestCapabilityContainsValue(t *testing.T) {
	caps := newCapabilitySet(DefaultCapabilities...)
	caps.Add(
		"urn:ietf:params:netconf:capability:with-defaults:1.0?basic-mode=report-all&also-supported=report-all-tagged,trim",
		"urn:ietf:params:netconf:capability:url:1.0?scheme=file,ftp,sftp,https",
	)
	assert.True(t, caps.Has(WithDefaultsCapability))
	assert.True(t, caps.Has(URLCapability))
	assert.True(t, caps.ContainsValue(WithDefaultsCapability, "also-supported", "report-all-tagged"))
	assert.True(t, caps.ContainsValue(WithDefaultsCapability, "basic-mode", "report-all"))
	assert.False(t, caps.ContainsValue(WithDefaultsCapability, "also-supported", "explicit"))
	assert.True(t, caps.ContainsValue(URLCapability, "scheme", "https"))
	assert.False(t, caps.ContainsValue(URLCapability, "scheme", "http"))
}
