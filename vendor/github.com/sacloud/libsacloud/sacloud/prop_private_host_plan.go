package sacloud

// propPrivateHostPlan 専有ホストプラン内包型
type propPrivateHostPlan struct {
	Plan *ProductPrivateHost `json:",omitempty"` // 専有ホストプラン
}

// GetPrivateHostPlan 専有ホストプラン取得
func (p *propPrivateHostPlan) GetPrivateHostPlan() *ProductPrivateHost {
	return p.Plan
}

// SetPrivateHostPlan 専有ホストプラン設定
func (p *propPrivateHostPlan) SetPrivateHostPlan(plan *ProductPrivateHost) {
	p.Plan = plan
}

// SetPrivateHostPlanByID 専有ホストプラン設定
func (p *propPrivateHostPlan) SetPrivateHostPlanByID(planID int64) {
	if p.Plan == nil {
		p.Plan = &ProductPrivateHost{}
	}
	p.Plan.Resource = NewResource(planID)
}

// GetCPU CPUコア数 取得
func (p *propPrivateHostPlan) GetCPU() int {
	if p.Plan == nil {
		return -1
	}

	return p.Plan.GetCPU()
}

// GetMemoryMB メモリ(MB) 取得
func (p *propPrivateHostPlan) GetMemoryMB() int {
	if p.Plan == nil {
		return -1
	}

	return p.Plan.GetMemoryMB()
}

// GetMemoryGB メモリ(GB) 取得
func (p *propPrivateHostPlan) GetMemoryGB() int {
	if p.Plan == nil {
		return -1
	}

	return p.Plan.GetMemoryGB()
}
