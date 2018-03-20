package sacloud

// IPv6Net IPv6ネットワーク(サブネット)
type IPv6Net struct {
	*Resource        // ID
	propScope        // スコープ
	propServiceClass // サービスクラス
	propCreatedAt    // 作成日時

	IPv6Prefix         string    `json:",omitempty"` // IPv6プレフィックス
	IPv6PrefixLen      int       `json:",omitempty"` // IPv6プレフィックス長
	IPv6PrefixTail     string    `json:",omitempty"` // IPv6プレフィックス末尾
	IPv6Table          *Resource `json:",omitempty"` // IPv6テーブル
	NamedIPv6AddrCount int       `json:",omitempty"` // 名前付きIPv6アドレス数
	ServiceID          int64     `json:",omitempty"` // サービスID
	Switch             *Switch   `json:",omitempty"` // 接続先スイッチ

}
