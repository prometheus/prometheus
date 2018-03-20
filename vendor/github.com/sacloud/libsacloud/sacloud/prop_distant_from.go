package sacloud

// propDistantFrom ストレージ隔離対象ディスク内包型
type propDistantFrom struct {
	DistantFrom []int64 `json:",omitempty"` // ストレージ隔離対象ディスク
}

// GetDistantFrom ストレージ隔離対象ディスク 取得
func (p *propDistantFrom) GetDistantFrom() []int64 {
	return p.DistantFrom
}

// SetDistantFrom ストレージ隔離対象ディスク 設定
func (p *propDistantFrom) SetDistantFrom(ids []int64) {
	p.DistantFrom = ids
}

// AddDistantFrom ストレージ隔離対象ディスク 追加
func (p *propDistantFrom) AddDistantFrom(id int64) {
	p.DistantFrom = append(p.DistantFrom, id)
}

// ClearDistantFrom ストレージ隔離対象ディスク クリア
func (p *propDistantFrom) ClearDistantFrom() {
	p.DistantFrom = []int64{}
}
