package sacloud

// VPCRouterStatus VPCルータのステータス情報
type VPCRouterStatus struct {
	FirewallReceiveLogs []string
	FirewallSendLogs    []string
	VPNLogs             []string
}
