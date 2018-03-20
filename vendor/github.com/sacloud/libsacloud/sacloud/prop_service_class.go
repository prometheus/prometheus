package sacloud

// propServiceClass サービスクラス内包型
type propServiceClass struct {
	ServiceClass string `json:",omitempty"` // サービスクラス
}

// GetServiceClass サービスクラス 取得
func (p *propServiceClass) GetServiceClass() string {
	return p.ServiceClass
}
