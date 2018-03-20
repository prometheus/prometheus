package sacloud

// propName 名称内包型
type propName struct {
	Name string `json:",omitempty"` // 名称
}

// GetName 名称 取得
func (p *propName) GetName() string {
	return p.Name
}

// SetName 名称 設定
func (p *propName) SetName(name string) {
	p.Name = name
}
