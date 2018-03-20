package sacloud

// propStorageClass ストレージクラス内包型
type propStorageClass struct {
	StorageClass string `json:",omitempty"` // ストレージクラス
}

// GetStorageClass ストレージクラス 取得
func (p *propStorageClass) GetStorageClass() string {
	return p.StorageClass
}
