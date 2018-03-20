package sacloud

// propScope スコープ内包型
type propScope struct {
	Scope string `json:",omitempty"` // スコープ
}

// GetScope スコープ 取得
func (p *propScope) GetScope() string {
	return p.Scope
}

// SetScope スコープ 設定
func (p *propScope) SetScope(scope string) {
	p.Scope = scope
}

// SetSharedScope 共有スコープに設定
func (p *propScope) SetSharedScope() {
	p.Scope = string(ESCopeShared)
}

// SetUserScope ユーザースコープに設定
func (p *propScope) SetUserScope() {
	p.Scope = string(ESCopeUser)
}

// IsSharedScope 共有スコープか判定
func (p *propScope) IsSharedScope() bool {
	if p == nil {
		return false
	}
	return p.Scope == string(ESCopeShared)
}

// IsUserScope ユーザースコープか判定
func (p *propScope) IsUserScope() bool {
	if p == nil {
		return false
	}
	return p.Scope == string(ESCopeUser)
}
