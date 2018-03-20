package sacloud

// propRegion リージョン内包型
type propRegion struct {
	Region *Region `json:",omitempty"` // リージョン
}

// GetRegion リージョン 取得
func (p *propRegion) GetRegion() *Region {
	return p.Region
}

// GetRegionID リージョンID 取得
func (p *propRegion) GetRegionID() int64 {
	if p.Region == nil {
		return -1
	}
	return p.Region.GetID()
}

// GetRegionName リージョン名 取得
func (p *propRegion) GetRegionName() string {
	if p.Region == nil {
		return ""
	}
	return p.Region.GetName()
}

// GetRegionDescription リージョン説明 取得
func (p *propRegion) GetRegionDescription() string {
	if p.Region == nil {
		return ""
	}
	return p.Region.GetDescription()
}

// GetRegionNameServers リージョンのネームサーバー(のIPアドレス)取得
func (p *propRegion) GetRegionNameServers() []string {
	if p.Region == nil {
		return []string{}
	}

	return p.Region.GetNameServers()
}
