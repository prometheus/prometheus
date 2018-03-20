package sacloud

// propServer 接続先サーバー内包型
type propServer struct {
	Server *Server `json:",omitempty"` // 接続先サーバー
}

// GetServer 接続先サーバー 取得
func (p *propServer) GetServer() *Server {
	return p.Server
}

// SetServer 接続先サーバー 設定
func (p *propServer) SetServer(server *Server) {
	p.Server = server
}

// SetServerID サーバーIDの設定
func (p *propServer) SetServerID(id int64) {
	p.Server = &Server{Resource: &Resource{ID: id}}
}
