package sacloud

// propInterfaces インターフェース(NIC)配列内包型
type propInterfaces struct {
	Interfaces []Interface `json:",omitempty"` // インターフェース
}

// GetInterfaces インターフェース(NIC)配列 取得
func (p *propInterfaces) GetInterfaces() []Interface {
	return p.Interfaces
}

// GetFirstInterface インターフェース(NIC)配列の先頭要素を返す
func (p *propInterfaces) GetFirstInterface() *Interface {
	if len(p.Interfaces) == 0 {
		return nil
	}
	return &p.Interfaces[0]
}
