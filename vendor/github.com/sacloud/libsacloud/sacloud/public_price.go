package sacloud

// PublicPrice 料金
type PublicPrice struct {
	DisplayName      string `json:",omitempty"` // 表示名
	IsPublic         bool   `json:",omitempty"` // 公開フラグ
	ServiceClassID   int    `json:",omitempty"` // サービスクラスID
	ServiceClassName string `json:",omitempty"` // サービスクラス名
	ServiceClassPath string `json:",omitempty"` // サービスクラスパス

	Price struct { // 価格
		Base    int    `json:",omitempty"` // 基本料金
		Daily   int    `json:",omitempty"` // 日単位料金
		Hourly  int    `json:",omitempty"` // 時間単位料金
		Monthly int    `json:",omitempty"` // 分単位料金
		Zone    string `json:",omitempty"` // ゾーン
	}
}
