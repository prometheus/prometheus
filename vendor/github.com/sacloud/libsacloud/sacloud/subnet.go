package sacloud

// Subnet IPv4サブネット
type Subnet struct {
	*Resource        // ID
	propServiceClass // サービスクラス
	propCreatedAt    // 作成日時

	DefaultRoute   string       `json:",omitempty"` // デフォルトルート
	IPAddresses    []*IPAddress `json:",omitempty"` // IPv4アドレス範囲
	NetworkAddress string       `json:",omitempty"` // ネットワークアドレス
	NetworkMaskLen int          `json:",omitempty"` // ネットワークマスク長
	ServiceID      int64        `json:",omitempty"` // サービスID
	StaticRoute    string       `json:",omitempty"` // スタティックルート
	NextHop        string       `json:",omitempty"` // ネクストホップ
	Switch         *Switch      `json:",omitempty"` // スイッチ
	Internet       *Internet    `json:",omitempty"` // ルーター
}
