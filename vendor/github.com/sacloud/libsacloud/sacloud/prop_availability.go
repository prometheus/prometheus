package sacloud

// propAvailability 有効状態内包型
type propAvailability struct {
	Availability EAvailability `json:",omitempty"` // 有効状態
}

// IsAvailable 有効状態が"有効"か判定
func (p *propAvailability) IsAvailable() bool {
	return p.Availability.IsAvailable()
}

// IsUploading 有効状態が"アップロード中"か判定
func (p *propAvailability) IsUploading() bool {
	return p.Availability.IsUploading()
}

// IsFailed 有効状態が"失敗"か判定
func (p *propAvailability) IsFailed() bool {
	return p.Availability.IsFailed()
}

// IsMigrating 有効状態が"マイグレーション中"か判定
func (p *propAvailability) IsMigrating() bool {
	return p.Availability.IsMigrating()
}
