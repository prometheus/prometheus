package sacloud

// AuthStatus 現在の認証状態
type AuthStatus struct {
	Account            *Account    // アカウント
	Member             *Member     // 会員情報
	AuthClass          EAuthClass  `json:",omitempty"`      // 認証クラス
	AuthMethod         EAuthMethod `json:",omitempty"`      // 認証方法
	ExternalPermission string      `json:",omitempty"`      // 他サービスへのアクセス権
	IsAPIKey           bool        `json:",omitempty"`      // APIキーでのアクセスフラグ
	OperationPenalty   string      `json:",omitempty"`      // オペレーションペナルティ
	Permission         EPermission `json:",omitempty"`      // 権限
	IsOk               bool        `json:"is_ok,omitempty"` // 結果

	// RESTFilter [unknown type] `json:",omitempty"`
	// User [unknown type] `json:",omitempty"`

}

// --------------------------------------------------------

// EAuthClass 認証種別
type EAuthClass string

var (
	// EAuthClassAccount アカウント認証
	EAuthClassAccount = EAuthClass("account")
)

// --------------------------------------------------------

// EAuthMethod 認証方法
type EAuthMethod string

var (
	// EAuthMethodAPIKey APIキー認証
	EAuthMethodAPIKey = EAuthMethod("apikey")
)

// --------------------------------------------------------

// EExternalPermission 他サービスへのアクセス権
type EExternalPermission string

var (
	// EExternalPermissionBill 請求情報
	EExternalPermissionBill = EExternalPermission("bill")
	// EExternalPermissionCDN ウェブアクセラレータ
	EExternalPermissionCDN = EExternalPermission("cdn")
)

// --------------------------------------------------------

// EOperationPenalty ペナルティ
type EOperationPenalty string

var (
	// EOperationPenaltyNone ペナルティなし
	EOperationPenaltyNone = EOperationPenalty("none")
)

// --------------------------------------------------------

// EPermission アクセスレベル
type EPermission string

var (
	// EPermissionCreate 作成・削除権限
	EPermissionCreate = EPermission("create")

	// EPermissionArrange 設定変更権限
	EPermissionArrange = EPermission("arrange")

	// EPermissionPower 電源操作権限
	EPermissionPower = EPermission("power")

	// EPermissionView リソース閲覧権限
	EPermissionView = EPermission("view")
)
