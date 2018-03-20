package sacloud

// Internet ルーター
type Internet struct {
	*Resource        // ID
	propName         // 名称
	propDescription  // 説明
	propScope        // スコープ
	propServiceClass // サービスクラス
	propSwitch       // 接続先スイッチ
	propIcon         // アイコン
	propTags         // タグ
	propCreatedAt    // 作成日時

	BandWidthMbps  int `json:",omitempty"` // 帯域
	NetworkMaskLen int `json:",omitempty"` // ネットワークマスク長

	//TODO Zone(API側起因のデータ型不一致のため)
	// ZoneType
}

// GetBandWidthMbps 帯域幅 取得
func (i *Internet) GetBandWidthMbps() int {
	return i.BandWidthMbps
}

// SetBandWidthMbps 帯域幅 設定
func (i *Internet) SetBandWidthMbps(v int) {
	i.BandWidthMbps = v
}

// GetNetworkMaskLen ネットワークマスク長 取得
func (i *Internet) GetNetworkMaskLen() int {
	return i.NetworkMaskLen
}

// SetNetworkMaskLen ネットワークマスク長 設定
func (i *Internet) SetNetworkMaskLen(v int) {
	i.NetworkMaskLen = v
}

// AllowInternetBandWidth 設定可能な帯域幅の値リスト
func AllowInternetBandWidth() []int {
	return []int{100, 250, 500, 1000, 1500, 2000, 2500, 3000}
}

// AllowInternetNetworkMaskLen 設定可能なネットワークマスク長の値リスト
func AllowInternetNetworkMaskLen() []int {
	return []int{26, 27, 28}
}
