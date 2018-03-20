package sacloud

// Storage ストレージ
type Storage struct {
	*Resource       // ID
	propName        // 名称
	propDescription // 説明
	propZone        // ゾーン

	Class    string   `json:",omitempty"` // クラス
	DiskPlan struct { // ディスクプラン
		*Resource        // ID
		propName         // 名称
		propStorageClass // ストレージクラス
	} `json:",omitempty"`

	//Capacity []string `json:",omitempty"`
}
