package sacloud

// propClass クラス内包型
type propClass struct {
	Class string `json:",omitempty"` // サービスクラス
}

// GetClass クラス 取得
func (p *propClass) GetClass() string {
	return p.Class
}
