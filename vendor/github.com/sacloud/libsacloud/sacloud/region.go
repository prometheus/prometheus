package sacloud

// Region リージョン
type Region struct {
	*Resource       // ID
	propName        // 名称
	propDescription // 説明

	NameServers []string `json:",omitempty"` // NameServers ネームサーバー
}

// GetNameServers リージョン内のネームサーバー取得
func (r *Region) GetNameServers() []string {
	if r == nil {
		return []string{}
	}
	return r.NameServers
}
