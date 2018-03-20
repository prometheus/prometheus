package sacloud

// NFS NFS
type NFS struct {
	*Appliance // アプライアンス共通属性

	Remark   *NFSRemark   `json:",omitempty"` // リマーク
	Settings *NFSSettings `json:",omitempty"` // NFS設定
}

// NFSRemark リマーク
type NFSRemark struct {
	*ApplianceRemarkBase
	propPlanID
	// TODO Zone
	//Zone *Resource
	//SourceAppliance *Resource // クローン元DB
}

// NFSSettings NFS設定リスト
type NFSSettings struct {
}

// NFSPlan NFSプラン
type NFSPlan int

var (
	// NFSPlan100G 100Gプラン
	NFSPlan100G = NFSPlan(100)
	// NFSPlan500G 500Gプラン
	NFSPlan500G = NFSPlan(500)
	// NFSPlan1T 1T(1024GB)プラン
	NFSPlan1T = NFSPlan(1024 * 1)
	// NFSPlan2T 2T(2048GB)プラン
	NFSPlan2T = NFSPlan(1024 * 2)
	// NFSPlan4T 4T(4096GB)プラン
	NFSPlan4T = NFSPlan(1024 * 4)
)

// AllowNFSPlans 指定可能なNFSプラン
func AllowNFSPlans() []int {
	return []int{
		int(NFSPlan100G),
		int(NFSPlan500G),
		int(NFSPlan1T),
		int(NFSPlan2T),
		int(NFSPlan4T),
	}
}

// CreateNFSValue NFS作成用パラメーター
type CreateNFSValue struct {
	SwitchID        string    // 接続先スイッチID
	Plan            NFSPlan   // プラン
	IPAddress       string    // IPアドレス
	MaskLen         int       // ネットワークマスク長
	DefaultRoute    string    // デフォルトルート
	Name            string    // 名称
	Description     string    // 説明
	Tags            []string  // タグ
	Icon            *Resource // アイコン
	SourceAppliance *Resource // クローン元NFS
}

// NewCreateNFSValue NFS作成用パラメーター
func NewCreateNFSValue() *CreateNFSValue {
	return &CreateNFSValue{
		Plan: NFSPlan100G,
	}
}

// NewNFS NFS作成(冗長化なし)
func NewNFS(values *CreateNFSValue) *NFS {

	if int(values.Plan) == 0 {
		values.Plan = NFSPlan100G
	}

	return &NFS{
		Appliance: &Appliance{
			Class:           "nfs",
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
		Remark: &NFSRemark{
			ApplianceRemarkBase: &ApplianceRemarkBase{
				Switch: &ApplianceRemarkSwitch{
					ID: values.SwitchID,
				},
				Network: &ApplianceRemarkNetwork{
					NetworkMaskLen: values.MaskLen,
					DefaultRoute:   values.DefaultRoute,
				},
				Servers: []interface{}{
					map[string]string{"IPAddress": values.IPAddress},
				},
			},
			propPlanID: propPlanID{Plan: &Resource{ID: int64(values.Plan)}},
			//SourceAppliance: values.SourceAppliance,
		},
	}

}
