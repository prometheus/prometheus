package sacloud

import "fmt"

// GSLB GSLB(CommonServiceItem)
type GSLB struct {
	*Resource        // ID
	propName         // 名称
	propDescription  // 説明
	propServiceClass // サービスクラス
	propIcon         // アイコン
	propTags         // タグ
	propCreatedAt    // 作成日時
	PropModifiedAt   // 変更日時

	Status   GSLBStatus   `json:",omitempty"` // ステータス
	Provider GSLBProvider `json:",omitempty"` // プロバイダ
	Settings GSLBSettings `json:",omitempty"` // GSLB設定

}

// GSLBSettings GSLB設定
type GSLBSettings struct {
	GSLB GSLBRecordSets `json:",omitempty"` // GSLB GSLBエントリー
}

// GSLBStatus GSLBステータス
type GSLBStatus struct {
	FQDN string `json:",omitempty"` // GSLBのFQDN
}

// GSLBProvider プロバイダ
type GSLBProvider struct {
	Class string `json:",omitempty"` // クラス
}

// CreateNewGSLB GSLB作成
func CreateNewGSLB(gslbName string) *GSLB {
	return &GSLB{
		Resource: &Resource{},
		propName: propName{Name: gslbName},
		Provider: GSLBProvider{
			Class: "gslb",
		},
		Settings: GSLBSettings{
			GSLB: GSLBRecordSets{
				DelayLoop:   10,
				HealthCheck: defaultGSLBHealthCheck,
				Weighted:    "True",
				Servers:     []GSLBServer{},
			},
		},
	}

}

// AllowGSLBHealthCheckProtocol GSLB監視プロトコルリスト
func AllowGSLBHealthCheckProtocol() []string {
	return []string{"http", "https", "ping", "tcp"}
}

// HasGSLBServer GSLB配下にサーバーを保持しているか判定
func (g *GSLB) HasGSLBServer() bool {
	return len(g.Settings.GSLB.Servers) > 0
}

// CreateGSLBServer GSLB配下のサーバーを作成
func (g *GSLB) CreateGSLBServer(ip string) *GSLBServer {
	return &GSLBServer{
		IPAddress: ip,
		Enabled:   "True",
		Weight:    "1",
	}
}

// AddGSLBServer GSLB配下にサーバーを追加
func (g *GSLB) AddGSLBServer(server *GSLBServer) {
	var isExist = false
	for i := range g.Settings.GSLB.Servers {
		if g.Settings.GSLB.Servers[i].IPAddress == server.IPAddress {
			g.Settings.GSLB.Servers[i].Enabled = server.Enabled
			g.Settings.GSLB.Servers[i].Weight = server.Weight
			isExist = true
		}
	}

	if !isExist {
		g.Settings.GSLB.Servers = append(g.Settings.GSLB.Servers, *server)
	}
}

// ClearGSLBServer GSLB配下のサーバーをクリア
func (g *GSLB) ClearGSLBServer() {
	g.Settings.GSLB.Servers = []GSLBServer{}
}

// GSLBRecordSets GSLBエントリー
type GSLBRecordSets struct {
	DelayLoop   int             `json:",omitempty"` // 監視間隔
	HealthCheck GSLBHealthCheck `json:",omitempty"` // ヘルスチェック
	Weighted    string          `json:",omitempty"` // ウェイト
	SorryServer string          `json:",omitempty"` // ソーリーサーバー
	Servers     []GSLBServer    // サーバー
}

// AddServer GSLB配下のサーバーを追加
func (g *GSLBRecordSets) AddServer(ip string) {
	var record GSLBServer
	var isExist = false
	for i := range g.Servers {
		if g.Servers[i].IPAddress == ip {
			isExist = true
		}
	}

	if !isExist {
		record = GSLBServer{
			IPAddress: ip,
			Enabled:   "True",
			Weight:    "1",
		}
		g.Servers = append(g.Servers, record)
	}
}

// DeleteServer GSLB配下のサーバーを削除
func (g *GSLBRecordSets) DeleteServer(ip string) {
	res := []GSLBServer{}
	for i := range g.Servers {
		if g.Servers[i].IPAddress != ip {
			res = append(res, g.Servers[i])
		}
	}

	g.Servers = res
}

// GSLBServer GSLB配下のサーバー
type GSLBServer struct {
	IPAddress string `json:",omitempty"` // IPアドレス
	Enabled   string `json:",omitempty"` // 有効/無効
	Weight    string `json:",omitempty"` // ウェイト

}

// GSLBHealthCheck ヘルスチェック
type GSLBHealthCheck struct {
	Protocol string `json:",omitempty"` // プロトコル
	Host     string `json:",omitempty"` // 対象ホスト
	Path     string `json:",omitempty"` // HTTP/HTTPSの場合のリクエストパス
	Status   string `json:",omitempty"` // 期待するステータスコード
	Port     string `json:",omitempty"` // ポート番号
}

var defaultGSLBHealthCheck = GSLBHealthCheck{
	Protocol: "http",
	Host:     "",
	Path:     "/",
	Status:   "200",
}

// SetHTTPHealthCheck HTTPヘルスチェック 設定
func (g *GSLB) SetHTTPHealthCheck(hostHeader string, path string, responseCode int) {
	g.Settings.GSLB.HealthCheck.Protocol = "http"
	g.Settings.GSLB.HealthCheck.Host = hostHeader
	g.Settings.GSLB.HealthCheck.Path = path
	g.Settings.GSLB.HealthCheck.Status = fmt.Sprintf("%d", responseCode)
	g.Settings.GSLB.HealthCheck.Port = ""

}

// SetHTTPSHealthCheck HTTPSヘルスチェック 設定
func (g *GSLB) SetHTTPSHealthCheck(hostHeader string, path string, responseCode int) {
	g.Settings.GSLB.HealthCheck.Protocol = "https"
	g.Settings.GSLB.HealthCheck.Host = hostHeader
	g.Settings.GSLB.HealthCheck.Path = path
	g.Settings.GSLB.HealthCheck.Status = fmt.Sprintf("%d", responseCode)
	g.Settings.GSLB.HealthCheck.Port = ""
}

// SetPingHealthCheck Pingヘルスチェック 設定
func (g *GSLB) SetPingHealthCheck() {
	g.Settings.GSLB.HealthCheck.Protocol = "ping"
	g.Settings.GSLB.HealthCheck.Host = ""
	g.Settings.GSLB.HealthCheck.Path = ""
	g.Settings.GSLB.HealthCheck.Status = ""
	g.Settings.GSLB.HealthCheck.Port = ""
}

// SetTCPHealthCheck TCPヘルスチェック 設定
func (g *GSLB) SetTCPHealthCheck(port int) {
	g.Settings.GSLB.HealthCheck.Protocol = "tcp"
	g.Settings.GSLB.HealthCheck.Host = ""
	g.Settings.GSLB.HealthCheck.Path = ""
	g.Settings.GSLB.HealthCheck.Status = ""
	g.Settings.GSLB.HealthCheck.Port = fmt.Sprintf("%d", port)
}

// SetDelayLoop 監視間隔秒数 設定
func (g *GSLB) SetDelayLoop(delayLoop int) {
	g.Settings.GSLB.DelayLoop = delayLoop
}

// SetWeightedEnable 重み付け応答 有効/無効 設定
func (g *GSLB) SetWeightedEnable(enable bool) {
	v := "True"
	if !enable {
		v = "False"
	}
	g.Settings.GSLB.Weighted = v
}

// SetSorryServer ソーリーサーバ 設定
func (g *GSLB) SetSorryServer(server string) {
	g.Settings.GSLB.SorryServer = server
}
