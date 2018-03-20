package sacloud

// propInterfaceDriver インターフェースドライバ内包型
type propInterfaceDriver struct {
	InterfaceDriver EInterfaceDriver `json:",omitempty"` // NIC
}

// SetInterfaceDriver インターフェースドライバ 設定
func (p *propInterfaceDriver) SetInterfaceDriver(v EInterfaceDriver) {
	p.InterfaceDriver = v
}

// GetInterfaceDriver インターフェースドライバ 取得
func (p *propInterfaceDriver) GetInterfaceDriver() EInterfaceDriver {
	return p.InterfaceDriver
}

// SetInterfaceDriverByString インターフェースドライバ 設定(文字列)
func (p *propInterfaceDriver) SetInterfaceDriverByString(v string) {
	p.InterfaceDriver = EInterfaceDriver(v)
}

// GetInterfaceDriverString インターフェースドライバ 取得(文字列)
func (p *propInterfaceDriver) GetInterfaceDriverString() string {
	return string(p.InterfaceDriver)
}
