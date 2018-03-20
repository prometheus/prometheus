package sacloud

// LoadBalancer ロードバランサー
type LoadBalancer struct {
	*Appliance // アプライアンス共通属性

	Remark   *LoadBalancerRemark   `json:",omitempty"` // リマーク
	Settings *LoadBalancerSettings `json:",omitempty"` // ロードバランサー設定
}

// LoadBalancerRemark リマーク
type LoadBalancerRemark struct {
	*ApplianceRemarkBase
	// TODO Zone
	//Zone *Resource
}

// LoadBalancerSettings ロードバランサー設定リスト
type LoadBalancerSettings struct {
	LoadBalancer []*LoadBalancerSetting `json:",omitempty"` // ロードバランサー設定リスト
}

// LoadBalancerSetting ロードバランサー仮想IP設定
type LoadBalancerSetting struct {
	VirtualIPAddress string                `json:",omitempty"` // 仮想IPアドレス
	Port             string                `json:",omitempty"` // ポート番号
	DelayLoop        string                `json:",omitempty"` // 監視間隔
	SorryServer      string                `json:",omitempty"` // ソーリーサーバー
	Servers          []*LoadBalancerServer `json:",omitempty"` // 仮想IP配下の実サーバー
}

// LoadBalancerServer 仮想IP設定配下のサーバー
type LoadBalancerServer struct {
	IPAddress   string                   `json:",omitempty"` // IPアドレス
	Port        string                   `json:",omitempty"` // ポート番号
	HealthCheck *LoadBalancerHealthCheck `json:",omitempty"` // ヘルスチェック
	Enabled     string                   `json:",omitempty"` // 有効/無効
	Status      string                   `json:",omitempty"` // ステータス
	ActiveConn  string                   `json:",omitempty"` // アクティブなコネクション
}

// LoadBalancerHealthCheck ヘルスチェック
type LoadBalancerHealthCheck struct {
	Protocol string `json:",omitempty"` // プロトコル
	Path     string `json:",omitempty"` // HTTP/HTTPSの場合のリクエストパス
	Status   string `json:",omitempty"` // HTTP/HTTPSの場合の期待するレスポンスコード
}

// LoadBalancerPlan ロードバランサープラン
type LoadBalancerPlan int

var (
	// LoadBalancerPlanStandard スタンダードプラン
	LoadBalancerPlanStandard = LoadBalancerPlan(1)
	// LoadBalancerPlanPremium プレミアムプラン
	LoadBalancerPlanPremium = LoadBalancerPlan(2)
)

// CreateLoadBalancerValue ロードバランサー作成用パラメーター
type CreateLoadBalancerValue struct {
	SwitchID     string           // 接続先スイッチID
	VRID         int              // VRID
	Plan         LoadBalancerPlan // プラン
	IPAddress1   string           // IPアドレス
	MaskLen      int              // ネットワークマスク長
	DefaultRoute string           // デフォルトルート
	Name         string           // 名称
	Description  string           // 説明
	Tags         []string         // タグ
	Icon         *Resource        // アイコン
}

// CreateDoubleLoadBalancerValue ロードバランサー(冗長化あり)作成用パラメーター
type CreateDoubleLoadBalancerValue struct {
	*CreateLoadBalancerValue
	IPAddress2 string // IPアドレス2
}

// AllowLoadBalancerHealthCheckProtocol ロードバランサーでのヘルスチェック対応プロトコルリスト
func AllowLoadBalancerHealthCheckProtocol() []string {
	return []string{"http", "https", "ping", "tcp"}
}

// CreateNewLoadBalancerSingle ロードバランサー作成(冗長化なし)
func CreateNewLoadBalancerSingle(values *CreateLoadBalancerValue, settings []*LoadBalancerSetting) (*LoadBalancer, error) {

	lb := &LoadBalancer{
		Appliance: &Appliance{
			Class:           "loadbalancer",
			propName:        propName{Name: values.Name},
			propDescription: propDescription{Description: values.Description},
			propTags:        propTags{Tags: values.Tags},
			propPlanID:      propPlanID{Plan: &Resource{ID: int64(values.Plan)}},
			propIcon: propIcon{
				&Icon{
					Resource: values.Icon,
				},
			},
		},
		Remark: &LoadBalancerRemark{
			ApplianceRemarkBase: &ApplianceRemarkBase{
				Switch: &ApplianceRemarkSwitch{
					ID: values.SwitchID,
				},
				VRRP: &ApplianceRemarkVRRP{
					VRID: values.VRID,
				},
				Network: &ApplianceRemarkNetwork{
					NetworkMaskLen: values.MaskLen,
					DefaultRoute:   values.DefaultRoute,
				},
				Servers: []interface{}{
					map[string]string{"IPAddress": values.IPAddress1},
				},
			},
		},
	}

	for _, s := range settings {
		lb.AddLoadBalancerSetting(s)
	}

	return lb, nil
}

// CreateNewLoadBalancerDouble ロードバランサー(冗長化あり)作成
func CreateNewLoadBalancerDouble(values *CreateDoubleLoadBalancerValue, settings []*LoadBalancerSetting) (*LoadBalancer, error) {
	lb, err := CreateNewLoadBalancerSingle(values.CreateLoadBalancerValue, settings)
	if err != nil {
		return nil, err
	}
	lb.Remark.Servers = append(lb.Remark.Servers, map[string]string{"IPAddress": values.IPAddress2})
	return lb, nil
}

// AddLoadBalancerSetting ロードバランサー仮想IP設定追加
//
// ロードバランサー設定は仮想IPアドレス単位で保持しています。
// 仮想IPを増やす場合にこのメソッドを利用します。
func (l *LoadBalancer) AddLoadBalancerSetting(setting *LoadBalancerSetting) {
	if l.Settings == nil {
		l.Settings = &LoadBalancerSettings{}
	}
	if l.Settings.LoadBalancer == nil {
		l.Settings.LoadBalancer = []*LoadBalancerSetting{}
	}
	l.Settings.LoadBalancer = append(l.Settings.LoadBalancer, setting)
}

// DeleteLoadBalancerSetting ロードバランサー仮想IP設定の削除
func (l *LoadBalancer) DeleteLoadBalancerSetting(vip string, port string) {
	res := []*LoadBalancerSetting{}
	for _, l := range l.Settings.LoadBalancer {
		if l.VirtualIPAddress != vip || l.Port != port {
			res = append(res, l)
		}
	}

	l.Settings.LoadBalancer = res
}

// AddServer 仮想IP設定配下へ実サーバーを追加
func (s *LoadBalancerSetting) AddServer(server *LoadBalancerServer) {
	if s.Servers == nil {
		s.Servers = []*LoadBalancerServer{}
	}
	s.Servers = append(s.Servers, server)
}

// DeleteServer 仮想IP設定配下の実サーバーを削除
func (s *LoadBalancerSetting) DeleteServer(ip string, port string) {
	res := []*LoadBalancerServer{}
	for _, server := range s.Servers {
		if server.IPAddress != ip || server.Port != port {
			res = append(res, server)
		}
	}

	s.Servers = res

}
