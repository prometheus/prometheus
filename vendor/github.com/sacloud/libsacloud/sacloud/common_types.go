package sacloud

import (
	"fmt"
	"strconv"
	"time"
)

// Resource IDを持つ、さくらのクラウド上のリソース
type Resource struct {
	ID int64 // ID
}

// ResourceIDHolder ID保持インターフェース
type ResourceIDHolder interface {
	SetID(int64)
	GetID() int64
}

// EmptyID 空ID
const EmptyID int64 = 0

// NewResource 新規リソース作成
func NewResource(id int64) *Resource {
	return &Resource{ID: id}
}

// NewResourceByStringID ID文字列からリソース作成
func NewResourceByStringID(id string) *Resource {
	intID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		panic(err)
	}
	return &Resource{ID: intID}
}

// SetID ID 設定
func (n *Resource) SetID(id int64) {
	n.ID = id
}

// GetID ID 取得
func (n *Resource) GetID() int64 {
	if n == nil {
		return -1
	}
	return n.ID
}

// GetStrID 文字列でID取得
func (n *Resource) GetStrID() string {
	if n == nil {
		return ""
	}
	return fmt.Sprintf("%d", n.ID)
}

// EAvailability 有効状態
type EAvailability string

var (
	// EAAvailable 有効
	EAAvailable = EAvailability("available")
	// EAUploading アップロード中
	EAUploading = EAvailability("uploading")
	// EAFailed 失敗
	EAFailed = EAvailability("failed")
	// EAMigrating マイグレーション中
	EAMigrating = EAvailability("migrating")
)

// IsAvailable 有効状態が"有効"か判定
func (e EAvailability) IsAvailable() bool {
	return e == EAAvailable
}

// IsUploading 有効状態が"アップロード中"か判定
func (e EAvailability) IsUploading() bool {
	return e == EAUploading
}

// IsFailed 有効状態が"失敗"か判定
func (e EAvailability) IsFailed() bool {
	return e == EAFailed
}

// IsMigrating 有効状態が"マイグレーション中"か判定
func (e EAvailability) IsMigrating() bool {
	return e == EAMigrating
}

// EInterfaceDriver インターフェースドライバ
type EInterfaceDriver string

var (
	// InterfaceDriverVirtIO virtio
	InterfaceDriverVirtIO = EInterfaceDriver("virtio")
	// InterfaceDriverE1000 e1000
	InterfaceDriverE1000 = EInterfaceDriver("e1000")
)

// EServerInstanceStatus サーバーインスタンスステータス
type EServerInstanceStatus struct {
	Status       string `json:",omitempty"` // 現在のステータス
	BeforeStatus string `json:",omitempty"` // 前のステータス
}

// IsUp インスタンスが起動しているか判定
func (e *EServerInstanceStatus) IsUp() bool {
	return e.Status == "up"
}

// IsDown インスタンスがダウンしているか確認
func (e *EServerInstanceStatus) IsDown() bool {
	return e.Status == "down"
}

// GetStatus ステータス 取得
func (e *EServerInstanceStatus) GetStatus() string {
	return e.Status
}

// GetBeforeStatus 以前のステータス 取得
func (e *EServerInstanceStatus) GetBeforeStatus() string {
	return e.BeforeStatus
}

// EScope スコープ
type EScope string

var (
	// ESCopeShared sharedスコープ
	ESCopeShared = EScope("shared")
	// ESCopeUser userスコープ
	ESCopeUser = EScope("user")
)

// EDiskConnection ディスク接続方法
type EDiskConnection string

// SakuraCloudResources さくらのクラウド上のリソース種別一覧
type SakuraCloudResources struct {
	Server          *Server             `json:",omitempty"`     // サーバー
	Disk            *Disk               `json:",omitempty"`     // ディスク
	Note            *Note               `json:",omitempty"`     // スタートアップスクリプト
	Archive         *Archive            `json:",omitempty"`     // アーカイブ
	PacketFilter    *PacketFilter       `json:",omitempty"`     // パケットフィルタ
	PrivateHost     *PrivateHost        `json:",omitempty"`     // 専有ホスト
	Bridge          *Bridge             `json:",omitempty"`     // ブリッジ
	Icon            *Icon               `json:",omitempty"`     // アイコン
	Image           *Image              `json:",omitempty"`     // 画像
	Interface       *Interface          `json:",omitempty"`     // インターフェース
	Internet        *Internet           `json:",omitempty"`     // ルーター
	IPAddress       *IPAddress          `json:",omitempty"`     // IPv4アドレス
	IPv6Addr        *IPv6Addr           `json:",omitempty"`     // IPv6アドレス
	IPv6Net         *IPv6Net            `json:",omitempty"`     // IPv6ネットワーク
	License         *License            `json:",omitempty"`     // ライセンス
	Switch          *Switch             `json:",omitempty"`     // スイッチ
	CDROM           *CDROM              `json:",omitempty"`     // ISOイメージ
	SSHKey          *SSHKey             `json:",omitempty"`     // 公開鍵
	Subnet          *Subnet             `json:",omitempty"`     // IPv4ネットワーク
	DiskPlan        *ProductDisk        `json:",omitempty"`     // ディスクプラン
	InternetPlan    *ProductInternet    `json:",omitempty"`     // ルータープラン
	LicenseInfo     *ProductLicense     `json:",omitempty"`     // ライセンス情報
	ServerPlan      *ProductServer      `json:",omitempty"`     // サーバープラン
	PrivateHostPlan *ProductPrivateHost `json:",omitempty"`     // 専有ホストプラン
	Region          *Region             `json:",omitempty"`     // リージョン
	Zone            *Zone               `json:",omitempty"`     // ゾーン
	FTPServer       *FTPServer          `json:",omitempty"`     // FTPサーバー情報
	WebAccelSite    *WebAccelSite       `json:"Site,omitempty"` // ウェブアクセラレータ サイト
	//REMARK: CommonServiceItemとApplianceはapiパッケージにて別途定義
}

// SakuraCloudResourceList さくらのクラウド上のリソース種別一覧(複数形)
type SakuraCloudResourceList struct {
	Servers          []Server             `json:",omitempty"`      // サーバー
	Disks            []Disk               `json:",omitempty"`      // ディスク
	Notes            []Note               `json:",omitempty"`      // スタートアップスクリプト
	Archives         []Archive            `json:",omitempty"`      // アーカイブ
	PacketFilters    []PacketFilter       `json:",omitempty"`      // パケットフィルタ
	PrivateHosts     []PrivateHost        `json:",omitempty"`      // 専有ホスト
	Bridges          []Bridge             `json:",omitempty"`      // ブリッジ
	Icons            []Icon               `json:",omitempty"`      // アイコン
	Interfaces       []Interface          `json:",omitempty"`      // インターフェース
	Internet         []Internet           `json:",omitempty"`      // ルーター
	IPAddress        []IPAddress          `json:",omitempty"`      // IPv4アドレス
	IPv6Addrs        []IPv6Addr           `json:",omitempty"`      // IPv6アドレス
	IPv6Nets         []IPv6Net            `json:",omitempty"`      // IPv6ネットワーク
	Licenses         []License            `json:",omitempty"`      // ライセンス
	Switches         []Switch             `json:",omitempty"`      // スイッチ
	CDROMs           []CDROM              `json:",omitempty"`      // ISOイメージ
	SSHKeys          []SSHKey             `json:",omitempty"`      // 公開鍵
	Subnets          []Subnet             `json:",omitempty"`      // IPv4ネットワーク
	DiskPlans        []ProductDisk        `json:",omitempty"`      // ディスクプラン
	InternetPlans    []ProductInternet    `json:",omitempty"`      // ルータープラン
	LicenseInfo      []ProductLicense     `json:",omitempty"`      // ライセンス情報
	ServerPlans      []ProductServer      `json:",omitempty"`      // サーバープラン
	PrivateHostPlans []ProductPrivateHost `json:",omitempty"`      // 専有ホストプラン
	Regions          []Region             `json:",omitempty"`      // リージョン
	Zones            []Zone               `json:",omitempty"`      // ゾーン
	ServiceClasses   []PublicPrice        `json:",omitempty"`      // サービスクラス(価格情報)
	WebAccelSites    []WebAccelSite       `json:"Sites,omitempty"` // ウェブアクセラレータ サイト

	//REMARK:CommonServiceItemとApplianceはapiパッケージにて別途定義
}

// Request APIリクエスト型
type Request struct {
	SakuraCloudResources                        // さくらのクラウドリソース
	From                 int                    `json:",omitempty"` // ページング FROM
	Count                int                    `json:",omitempty"` // 取得件数
	Sort                 []string               `json:",omitempty"` // ソート
	Filter               map[string]interface{} `json:",omitempty"` // フィルタ
	Exclude              []string               `json:",omitempty"` // 除外する項目
	Include              []string               `json:",omitempty"` // 取得する項目

}

// AddFilter フィルタの追加
func (r *Request) AddFilter(key string, value interface{}) *Request {
	if r.Filter == nil {
		r.Filter = map[string]interface{}{}
	}
	r.Filter[key] = value
	return r
}

// AddSort ソートの追加
func (r *Request) AddSort(keyName string) *Request {
	if r.Sort == nil {
		r.Sort = []string{}
	}
	r.Sort = append(r.Sort, keyName)
	return r
}

// AddExclude 除外対象の追加
func (r *Request) AddExclude(keyName string) *Request {
	if r.Exclude == nil {
		r.Exclude = []string{}
	}
	r.Exclude = append(r.Exclude, keyName)
	return r
}

// AddInclude 選択対象の追加
func (r *Request) AddInclude(keyName string) *Request {
	if r.Include == nil {
		r.Include = []string{}
	}
	r.Include = append(r.Include, keyName)
	return r
}

// ResultFlagValue レスポンス値でのフラグ項目
type ResultFlagValue struct {
	IsOk    bool `json:"is_ok,omitempty"` // is_ok項目
	Success bool `json:",omitempty"`      // success項目
}

// SearchResponse 検索レスポンス
type SearchResponse struct {
	Total                    int        `json:",omitempty"` // トータル件数
	From                     int        `json:",omitempty"` // ページング開始ページ
	Count                    int        `json:",omitempty"` // 件数
	ResponsedAt              *time.Time `json:",omitempty"` // 応答日時
	*SakuraCloudResourceList            // さくらのクラウドリソース(複数形)
}

// Response レスポンス型
type Response struct {
	*ResultFlagValue      // フラグ値
	*SakuraCloudResources // さくらのクラウドリソース(単数形)
}

// ResultErrorValue レスポンスエラー型
type ResultErrorValue struct {
	IsFatal      bool   `json:"is_fatal,omitempty"`   // IsFatal
	Serial       string `json:"serial,omitempty"`     // Serial
	Status       string `json:"status,omitempty"`     // Status
	ErrorCode    string `json:"error_code,omitempty"` // ErrorCode
	ErrorMessage string `json:"error_msg,omitempty"`  // ErrorMessage

}

// MigrationJobStatus マイグレーションジョブステータス
type MigrationJobStatus struct {
	Status string `json:",omitempty"` // ステータス

	Delays *struct { // Delays
		Start *struct { // 開始
			Max int `json:",omitempty"` // 最大
			Min int `json:",omitempty"` // 最小
		} `json:",omitempty"`

		Finish *struct { // 終了
			Max int `json:",omitempty"` // 最大
			Min int `json:",omitempty"` // 最小
		} `json:",omitempty"`
	}
}

var (
	// TagGroupA サーバをグループ化し起動ホストを分離します(グループA)
	TagGroupA = "@group=a"
	// TagGroupB サーバをグループ化し起動ホストを分離します(グループB)
	TagGroupB = "@group=b"
	// TagGroupC サーバをグループ化し起動ホストを分離します(グループC)
	TagGroupC = "@group=b"
	// TagGroupD サーバをグループ化し起動ホストを分離します(グループD)
	TagGroupD = "@group=b"

	// TagAutoReboot サーバ停止時に自動起動します
	TagAutoReboot = "@auto-reboot"

	// TagKeyboardUS リモートスクリーン画面でUSキーボード入力します
	TagKeyboardUS = "@keyboard-us"

	// TagBootCDROM 優先ブートデバイスをCD-ROMに設定します
	TagBootCDROM = "@boot-cdrom"
	// TagBootNetwork 優先ブートデバイスをPXE bootに設定します
	TagBootNetwork = "@boot-network"
)

// DatetimeLayout さくらのクラウドAPIで利用される日付型のレイアウト(RFC3339)
var DatetimeLayout = "2006-01-02T15:04:05-07:00"
