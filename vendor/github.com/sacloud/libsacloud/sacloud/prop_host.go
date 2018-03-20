package sacloud

// propHost ホスト(物理)内包型
type propHost struct {
	Host *Host `json:",omitempty"` // サービスクラス
}

// GetHost ホスト(物理) 取得
func (p *propHost) GetHost() *Host {
	return p.Host
}

// GetHostName ホスト(物理)名称取得
func (p *propHost) GetHostName() string {
	if p.Host == nil {
		return ""
	}
	return p.Host.GetName()
}
