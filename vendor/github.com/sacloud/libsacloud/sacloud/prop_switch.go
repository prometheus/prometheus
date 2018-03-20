package sacloud

// propSwitch スイッチ内包型
type propSwitch struct {
	Switch *Switch `json:",omitempty"` // スイッチ
}

// GetSwitch スイッチ 取得
func (p *propSwitch) GetSwitch() *Switch {
	return p.Switch
}

// SetSwitch スイッチ 設定
func (p *propSwitch) SetSwitch(sw *Switch) {
	p.Switch = sw
}

// SetSwitchID スイッチID 設定
func (p *Interface) SetSwitchID(id int64) {
	p.Switch = &Switch{Resource: &Resource{ID: id}}
}
