package sacloud

// propStorage ストーレジ内包型
type propStorage struct {
	Storage *Storage `json:",omitempty"` // ストレージ
}

// GetStorage ストレージ 取得
func (p *propStorage) GetStorage() *Storage {
	return p.Storage
}
