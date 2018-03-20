package sacloud

// propPlanID プランID内包型
type propPlanID struct {
	Plan *Resource `json:",omitempty"` // プラン
}

// GetPlanID プランID 取得
func (p *propPlanID) GetPlanID() int64 {
	if p.Plan == nil {
		return -1
	}
	return p.Plan.GetID()
}

// GetStrPlanID プランID(文字列) 取得
func (p *propPlanID) GetStrPlanID() string {
	if p.Plan == nil {
		return ""
	}
	return p.Plan.GetStrID()
}
