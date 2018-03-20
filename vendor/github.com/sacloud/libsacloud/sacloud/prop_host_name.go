package sacloud

// propHostName ホスト名内包型
type propHostName struct {
	HostName string `json:",omitempty"` // ホスト名 (ディスクの修正実施時に指定した初期ホスト名)
}

// GetHostName (初期)ホスト名 取得
func (p *propHostName) GetHostName() string {
	return p.HostName
}
