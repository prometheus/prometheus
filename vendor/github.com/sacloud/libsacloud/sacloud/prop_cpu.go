package sacloud

// propCPU CPUコア数内包型
type propCPU struct {
	CPU int `json:",omitempty"` // CPUコア数
}

// GetCPU CPUコア数 取得
func (p *propCPU) GetCPU() int {
	return p.CPU
}
