package sacloud

// IPv6Addr IPアドレス(IPv6)
type IPv6Addr struct {
	HostName  string     `json:",omitempty"` // ホスト名
	IPv6Addr  string     `json:",omitempty"` // IPv6アドレス
	Interface *Interface `json:",omitempty"` // インターフェース
	IPv6Net   *IPv6Net   `json:",omitempty"` // IPv6サブネット

}

// GetIPv6NetID IPv6アドレスが所属するIPv6NetのIDを取得
func (a *IPv6Addr) GetIPv6NetID() int64 {
	if a.IPv6Net != nil {
		return a.IPv6Net.ID
	}
	return 0
}

// GetInternetID IPv6アドレスを所有するルータ+スイッチ(Internet)のIDを取得
func (a *IPv6Addr) GetInternetID() int64 {
	if a.IPv6Net != nil && a.IPv6Net.Switch != nil && a.IPv6Net.Switch.Internet != nil {
		return a.IPv6Net.Switch.Internet.ID
	}
	return 0
}

// CreateNewIPv6Addr IPv6アドレス作成
func CreateNewIPv6Addr() *IPv6Addr {
	return &IPv6Addr{
		IPv6Net: &IPv6Net{
			Resource: &Resource{},
		},
	}
}
