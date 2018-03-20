package sacloud

// propPrivateHost 専有ホスト内包型
type propPrivateHost struct {
	PrivateHost *PrivateHost // 専有ホスト
}

// SetPrivateHostByID 指定のアイコンIDを設定
func (p *propPrivateHost) SetPrivateHostByID(id int64) {
	p.PrivateHost = &PrivateHost{Resource: NewResource(id)}
}

// SetPrivateHost 指定のアイコンオブジェクトを設定
func (p *propPrivateHost) SetPrivateHost(icon *PrivateHost) {
	p.PrivateHost = icon
}

// ClearPrivateHost アイコンをクリア(空IDを持つアイコンオブジェクトをセット)
func (p *propPrivateHost) ClearPrivateHost() {
	p.PrivateHost = &PrivateHost{Resource: NewResource(EmptyID)}
}
