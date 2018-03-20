package sacloud

// propDiskConnection ディスク接続情報内包型
type propDiskConnection struct {
	Connection      EDiskConnection `json:",omitempty"` // ディスク接続方法
	ConnectionOrder int             `json:",omitempty"` // コネクション順序

}

// GetDiskConnection ディスク接続方法 取得
func (p *propDiskConnection) GetDiskConnection() EDiskConnection {
	return p.Connection
}

// SetDiskConnection ディスク接続方法 設定
func (p *propDiskConnection) SetDiskConnection(conn EDiskConnection) {
	p.Connection = conn
}

// GetDiskConnectionByStr ディスク接続方法 取得
func (p *propDiskConnection) GetDiskConnectionByStr() string {
	return string(p.Connection)
}

// SetDiskConnectionByStr ディスク接続方法 設定
func (p *propDiskConnection) SetDiskConnectionByStr(conn string) {
	p.Connection = EDiskConnection(conn)
}

// GetDiskConnectionOrder コネクション順序 取得
func (p *propDiskConnection) GetDiskConnectionOrder() int {
	return p.ConnectionOrder
}
