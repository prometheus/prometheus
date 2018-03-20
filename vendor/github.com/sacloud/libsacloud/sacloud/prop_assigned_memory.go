package sacloud

// propAssignedMemoryMB サイズ(MB)内包型
type propAssignedMemoryMB struct {
	AssignedMemoryMB int `json:",omitempty"` // サイズ(MB単位)
}

// GetAssignedMemoryMB サイズ(MB単位) 取得
func (p *propAssignedMemoryMB) GetAssignedMemoryMB() int {
	return p.AssignedMemoryMB
}

// GetAssignedMemoryGB サイズ(GB単位) 取得
func (p *propAssignedMemoryMB) GetAssignedMemoryGB() int {
	if p.AssignedMemoryMB <= 0 {
		return 0
	}
	return p.AssignedMemoryMB / 1024
}
