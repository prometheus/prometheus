package sacloud

// IPAddress IPアドレス(IPv4)
type IPAddress struct {
	HostName  string     `json:",omitempty"` // HostName ホスト名
	IPAddress string     `json:",omitempty"` // IPAddress IPv4アドレス
	Interface *Interface `json:",omitempty"` // Interface インターフェース
	Subnet    *Subnet    `json:",omitempty"` // Subnet IPv4サブネット

}
