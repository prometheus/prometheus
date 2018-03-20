package sacloud

// propDescription 説明内包型
type propDescription struct {
	Description string // 説明
}

// GetDescription 説明 取得
func (p *propDescription) GetDescription() string {
	return p.Description
}

// SetDescription 説明 設定
func (p *propDescription) SetDescription(desc string) {
	p.Description = desc
}
