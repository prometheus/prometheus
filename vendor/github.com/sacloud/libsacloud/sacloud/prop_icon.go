package sacloud

// propIcon アイコン内包型
type propIcon struct {
	Icon *Icon // アイコン
}

// GetIcon アイコンを取得
func (p *propIcon) GetIcon() *Icon {
	return p.Icon
}

// GetIconID アイコンIDを取得
func (p *propIcon) GetIconID() int64 {
	if p.HasIcon() {
		return p.Icon.GetID()
	}
	return -1
}

// GetIconStrID アイコンID(文字列)を取得
func (p *propIcon) GetIconStrID() string {
	if p.HasIcon() {
		return p.Icon.GetStrID()
	}
	return ""
}

// HasIcon アイコンがセットされているか
func (p *propIcon) HasIcon() bool {
	return p.Icon != nil
}

// SetIconByID 指定のアイコンIDを設定
func (p *propIcon) SetIconByID(id int64) {
	p.Icon = &Icon{Resource: NewResource(id)}
}

// SetIcon 指定のアイコンオブジェクトを設定
func (p *propIcon) SetIcon(icon *Icon) {
	p.Icon = icon
}

// ClearIcon アイコンをクリア(空IDを持つアイコンオブジェクトをセット)
func (p *propIcon) ClearIcon() {
	p.Icon = &Icon{Resource: NewResource(EmptyID)}
}
