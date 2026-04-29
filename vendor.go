package netconf

// CiscoNXOSAttrs provides the namespace attributes required by Cisco NX-OS
// legacy NX-API XML NETCONF stack (port 22, "feature nxapi").
var CiscoNXOSAttrs = []SessionOption{
	WithRPCAttr("xmlns:nxos", "http://www.cisco.com/nxos:1.0"),
	WithRPCAttr("xmlns:if", "http://www.cisco.com/nxos:1.0:if_manager"),
	WithRPCAttr("xmlns:nfcli", "http://www.cisco.com/nxos:1.0:nfcli"),
	WithRPCAttr("xmlns:vlan_mgr_cli", "http://www.cisco.com/nxos:1.0:vlan_mgr_cli"),
}

// HPComwareAttrs provides the namespace attributes used by HP Comware devices.
var HPComwareAttrs = []SessionOption{
	WithRPCAttr("xmlns:data", "http://www.hp.com/netconf/data:1.0"),
	WithRPCAttr("xmlns:config", "http://www.hp.com/netconf/config:1.0"),
}
