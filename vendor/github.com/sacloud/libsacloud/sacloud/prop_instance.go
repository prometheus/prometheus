package sacloud

// propInstance インスタンス内包型
type propInstance struct {
	Instance *Instance `json:",omitempty"` // インスタンス
}

// GetInstance インスタンス 取得
func (p *propInstance) GetInstance() *Instance {
	return p.Instance
}

// IsUp インスタンスが起動しているか判定
func (p *propInstance) IsUp() bool {
	if p.Instance == nil {
		return false
	}
	return p.Instance.IsUp()
}

// IsDown インスタンスがダウンしているか確認
func (p *propInstance) IsDown() bool {
	if p.Instance == nil {
		return false
	}
	return p.Instance.IsDown()
}

// GetInstanceStatus ステータス 取得
func (p *propInstance) GetInstanceStatus() string {
	if p.Instance == nil {
		return ""
	}
	return p.Instance.GetStatus()
}

// GetInstanceBeforeStatus 以前のステータス 取得
func (p *propInstance) GetInstanceBeforeStatus() string {
	if p.Instance == nil {
		return ""
	}
	return p.Instance.GetBeforeStatus()
}

// MaintenanceScheduled メンテナンス予定の有無
func (p *propInstance) MaintenanceScheduled() bool {
	if p.Instance == nil {
		return false
	}
	return p.Instance.MaintenanceScheduled()
}

// GetMaintenanceInfoURL メンテナンス情報 URL取得
func (p *propInstance) GetMaintenanceInfoURL() string {
	if p.Instance == nil {
		return ""
	}
	return p.Instance.Host.InfoURL
}
