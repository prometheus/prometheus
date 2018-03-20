package sacloud

// Server サーバー
type Server struct {
	*Resource             // ID
	propName              // 名称
	propDescription       // 説明
	propHostName          // ホスト名
	propInterfaceDriver   // インターフェースドライバ
	propAvailability      // 有功状態
	propServerPlan        // サーバープラン
	propZone              // ゾーン
	propServiceClass      // サービスクラス
	propConnectedSwitches // 接続スイッチ
	propDisks             // ディスク配列
	propInstance          // インスタンス
	propInterfaces        // インターフェース配列
	propPrivateHost       // 専有ホスト
	propIcon              // アイコン
	propTags              // タグ
	propCreatedAt         // 作成日時
}

const (
	// ServerMaxInterfaceLen サーバーに接続できるNICの最大数
	ServerMaxInterfaceLen = 10
	// ServerMaxDiskLen サーバーに接続できるディスクの最大数
	ServerMaxDiskLen = 4
)

// KeyboardRequest キーボード送信リクエスト
type KeyboardRequest struct {
	Keys []string `json:",omitempty"` // キー(複数)
	Key  string   `json:",omitempty"` // キー(単体)
}

// MouseRequest マウス送信リクエスト
type MouseRequest struct {
	X       *int                 `json:",omitempty"` // X
	Y       *int                 `json:",omitempty"` // Y
	Z       *int                 `json:",omitempty"` // Z
	Buttons *MouseRequestButtons `json:",omitempty"` // マウスボタン

}

// VNCSnapshotRequest VNCスナップショット取得リクエスト
type VNCSnapshotRequest struct {
	ScreenSaverExitTimeMS int `json:",omitempty"` // スクリーンセーバーからの復帰待ち時間
}

// MouseRequestButtons マウスボタン
type MouseRequestButtons struct {
	L bool `json:",omitempty"` // 左ボタン
	R bool `json:",omitempty"` // 右ボタン
	M bool `json:",omitempty"` // 中ボタン
}

// VNCProxyResponse VNCプロキシ取得レスポンス
type VNCProxyResponse struct {
	*ResultFlagValue
	Status       string `json:",omitempty"` // ステータス
	Host         string `json:",omitempty"` // プロキシホスト
	IOServerHost string `json:",omitempty"` // 新プロキシホスト(Hostがlocalhostの場合にこちらを利用する)
	Port         string `json:",omitempty"` // ポート番号
	Password     string `json:",omitempty"` // VNCパスワード
	VNCFile      string `json:",omitempty"` // VNC接続情報ファイル(VNCビューア用)
}

// ActualHost プロキシホスト名(Host or IOServerHost)を返す
func (r *VNCProxyResponse) ActualHost() string {
	host := r.Host
	if host == "localhost" {
		host = r.IOServerHost
	}
	return host
}

// VNCSizeResponse VNC画面サイズレスポンス
type VNCSizeResponse struct {
	Width  int `json:",string,omitempty"` // 幅
	Height int `json:",string,omitempty"` // 高さ
}

// VNCSnapshotResponse VPCスナップショットレスポンス
type VNCSnapshotResponse struct {
	Image string `json:",omitempty"` // スナップショット画像データ
}
