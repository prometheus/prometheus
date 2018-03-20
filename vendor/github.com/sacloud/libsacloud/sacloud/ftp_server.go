package sacloud

// FTPServer FTPサーバー接続情報
type FTPServer struct {
	HostName  string `json:",omitempty"` // FTPサーバーホスト名
	IPAddress string `json:",omitempty"` // FTPサーバー IPアドレス
	User      string `json:",omitempty"` // 接続ユーザー名
	Password  string `json:",omitempty"` // パスワード

}

// FTPOpenRequest FTP接続オープンリクエスト
type FTPOpenRequest struct {
	ChangePassword bool // パスワード変更フラグ
}
