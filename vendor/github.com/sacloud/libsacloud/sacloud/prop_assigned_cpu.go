package sacloud

// propAssignedCPU CPUコア数内包型
type propAssignedCPU struct {
	AssignedCPU int `json:",omitempty"` // CPUコア数
}

// GetAssignedCPU CPUコア数 取得
func (p *propAssignedCPU) GetAssignedCPU() int {
	return p.AssignedCPU
}
