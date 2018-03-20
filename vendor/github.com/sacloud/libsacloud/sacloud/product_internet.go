package sacloud

// ProductInternet ルータープラン
type ProductInternet struct {
	*Resource        // ID
	propName         // 名称
	propAvailability // 有功状態
	propServiceClass // サービスクラス

	BandWidthMbps int `json:",omitempty"` // 帯域幅

}
