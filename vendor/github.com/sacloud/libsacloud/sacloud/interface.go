package sacloud

// Interface インターフェース(NIC)
type Interface struct {
	*Resource                   // ID
	propServer                  // サーバー
	propSwitch                  // スイッチ
	MACAddress    string        `json:",omitempty"` // MACアドレス
	IPAddress     string        `json:",omitempty"` // IPアドレス
	UserIPAddress string        `json:",omitempty"` // ユーザー指定IPアドレス
	HostName      string        `json:",omitempty"` // ホスト名
	PacketFilter  *PacketFilter `json:",omitempty"` // 適用パケットフィルタ
}

// GetMACAddress MACアドレス 取得
func (i *Interface) GetMACAddress() string {
	return i.MACAddress
}

//GetIPAddress IPアドレス 取得
func (i *Interface) GetIPAddress() string {
	return i.IPAddress
}

// SetUserIPAddress ユーザー指定IPアドレス 設定
func (i *Interface) SetUserIPAddress(ip string) {
	i.UserIPAddress = ip
}

//GetUserIPAddress ユーザー指定IPアドレス 取得
func (i *Interface) GetUserIPAddress() string {
	return i.UserIPAddress
}

// GetHostName ホスト名 取得
func (i *Interface) GetHostName() string {
	return i.HostName
}

// GetPacketFilter 適用パケットフィルタ 取得
func (i *Interface) GetPacketFilter() *PacketFilter {
	return i.PacketFilter
}
