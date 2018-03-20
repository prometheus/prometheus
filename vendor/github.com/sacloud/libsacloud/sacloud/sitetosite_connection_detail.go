package sacloud

// SiteToSiteConnectionDetail サイト間VPN接続詳細情報
type SiteToSiteConnectionDetail struct {
	ESP struct {
		AuthenticationProtocol string
		DHGroup                string
		EncryptionProtocol     string
		Lifetime               string
		Mode                   string
		PerfectForwardSecrecy  string
	}
	IKE struct {
		AuthenticationProtocol string
		EncryptionProtocol     string
		Lifetime               string
		Mode                   string
		PerfectForwardSecrecy  string
		PreSharedSecret        string
	}
	Peer struct {
		ID               string
		InsideNetworks   []string
		OutsideIPAddress string
	}
	VPCRouter struct {
		ID               string
		InsideNetworks   []string
		OutsideIPAddress string
	}
}

// SiteToSiteConnectionInfo サイト間VPN接続情報
type SiteToSiteConnectionInfo struct {
	Details struct {
		Config []SiteToSiteConnectionDetail
	}
}
