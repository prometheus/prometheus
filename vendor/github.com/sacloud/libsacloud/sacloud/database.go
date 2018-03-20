package sacloud

import (
	"encoding/json"
	"strings"
)

// Database データベース(appliance)
type Database struct {
	*Appliance // アプライアンス共通属性

	Remark   *DatabaseRemark   `json:",omitempty"` // リマーク
	Settings *DatabaseSettings `json:",omitempty"` // データベース設定
}

// DatabaseRemark データベースリマーク
type DatabaseRemark struct {
	*ApplianceRemarkBase
	propPlanID                             // プランID
	DBConf          *DatabaseCommonRemarks // コンフィグ
	Network         *DatabaseRemarkNetwork // ネットワーク
	SourceAppliance *Resource              // クローン元DB
	Zone            struct {               // ゾーン
		ID json.Number `json:",omitempty"` // ゾーンID
	}
}

// DatabaseRemarkNetwork ネットワーク
type DatabaseRemarkNetwork struct {
	NetworkMaskLen int    `json:",omitempty"` // ネットワークマスク長
	DefaultRoute   string `json:",omitempty"` // デフォルトルート
}

// UnmarshalJSON JSONアンマーシャル(配列、オブジェクトが混在するためここで対応)
func (s *DatabaseRemarkNetwork) UnmarshalJSON(data []byte) error {
	targetData := strings.Replace(strings.Replace(string(data), " ", "", -1), "\n", "", -1)
	if targetData == `[]` {
		return nil
	}

	tmp := &struct {
		// NetworkMaskLen
		NetworkMaskLen int `json:",omitempty"`
		// DefaultRoute
		DefaultRoute string `json:",omitempty"`
	}{}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	s.NetworkMaskLen = tmp.NetworkMaskLen
	s.DefaultRoute = tmp.DefaultRoute
	return nil
}

// DatabaseCommonRemarks リマークリスト
type DatabaseCommonRemarks struct {
	Common *DatabaseCommonRemark // Common
}

// DatabaseCommonRemark リマーク
type DatabaseCommonRemark struct {
	DatabaseName     string `json:",omitempty"` // 名称
	DatabaseRevision string `json:",omitempty"` // リビジョン
	DatabaseTitle    string `json:",omitempty"` // タイトル
	DatabaseVersion  string `json:",omitempty"` // バージョン
	ReplicaPassword  string `json:",omitempty"` // レプリケーションパスワード
	ReplicaUser      string `json:",omitempty"` // レプリケーションユーザー
}

// DatabaseSettings データベース設定リスト
type DatabaseSettings struct {
	DBConf *DatabaseSetting `json:",omitempty"` // コンフィグ
}

// DatabaseSetting データベース設定
type DatabaseSetting struct {
	Backup *DatabaseBackupSetting `json:",omitempty"` // バックアップ設定
	Common *DatabaseCommonSetting `json:",oitempty"`  // 共通設定
}

// DatabaseServer データベースサーバー情報
type DatabaseServer struct {
	IPAddress  string `json:",omitempty"` // IPアドレス
	Port       string `json:",omitempty"` // ポート
	Enabled    string `json:",omitempty"` // 有効/無効
	Status     string `json:",omitempty"` // ステータス
	ActiveConn string `json:",omitempty"` // アクティブコネクション
}

// DatabasePlan プラン
type DatabasePlan int

var (
	// DatabasePlanMini ミニプラン(後方互換用)
	DatabasePlanMini = DatabasePlan(10)
	// DatabasePlan10G 10Gプラン
	DatabasePlan10G = DatabasePlan(10)
	// DatabasePlan30G 30Gプラン
	DatabasePlan30G = DatabasePlan(30)
	// DatabasePlan90G 90Gプラン
	DatabasePlan90G = DatabasePlan(90)
	// DatabasePlan240G 240Gプラン
	DatabasePlan240G = DatabasePlan(240)
)

// AllowDatabasePlans 指定可能なデータベースプラン
func AllowDatabasePlans() []int {
	return []int{
		int(DatabasePlan10G),
		int(DatabasePlan30G),
		int(DatabasePlan90G),
		int(DatabasePlan240G),
	}
}

// DatabaseBackupSetting バックアップ設定
type DatabaseBackupSetting struct {
	Rotate int    `json:",omitempty"` // ローテーション世代数
	Time   string `json:",omitempty"` // 開始時刻
}

// DatabaseCommonSetting 共通設定
type DatabaseCommonSetting struct {
	DefaultUser   string        `json:",omitempty"` // ユーザー名
	UserPassword  string        `json:",omitempty"` // ユーザーパスワード
	WebUI         interface{}   `json:",omitempty"` // WebUIのIPアドレス or FQDN
	ServicePort   string        // ポート番号
	SourceNetwork SourceNetwork // 接続許可ネットワーク
}

// SourceNetwork 接続許可ネットワーク
type SourceNetwork []string

// UnmarshalJSON JSONアンマーシャル(配列と文字列が混在するためここで対応)
func (s *SourceNetwork) UnmarshalJSON(data []byte) error {
	// SourceNetworkが未設定の場合、APIレスポンスが""となるため回避する
	if string(data) == `""` {
		return nil
	}

	tmp := []string{}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	source := SourceNetwork(tmp)
	*s = source
	return nil
}

// MarshalJSON JSONマーシャル(配列と文字列が混在するためここで対応)
func (s *SourceNetwork) MarshalJSON() ([]byte, error) {
	if s == nil {
		return []byte(""), nil
	}

	list := []string(*s)
	if len(list) == 0 || (len(list) == 1 && list[0] == "") {
		return []byte(`""`), nil
	}

	return json.Marshal(list)
}

// CreateDatabaseValue データベース作成用パラメータ
type CreateDatabaseValue struct {
	Plan          DatabasePlan // プラン
	AdminPassword string       // 管理者パスワード
	DefaultUser   string       // ユーザー名
	UserPassword  string       // パスワード
	SourceNetwork []string     // 接続許可ネットワーク
	ServicePort   string       // ポート
	// BackupRotate     int          // バックアップ世代数
	BackupTime       string    // バックアップ開始時間
	SwitchID         string    // 接続先スイッチ
	IPAddress1       string    // IPアドレス1
	MaskLen          int       // ネットワークマスク長
	DefaultRoute     string    // デフォルトルート
	Name             string    // 名称
	Description      string    // 説明
	Tags             []string  // タグ
	Icon             *Resource // アイコン
	WebUI            bool      // WebUI有効
	DatabaseName     string    // データベース名
	DatabaseRevision string    // リビジョン
	DatabaseTitle    string    // データベースタイトル
	DatabaseVersion  string    // データベースバージョン
	ReplicaUser      string    // ReplicaUser レプリケーションユーザー
	SourceAppliance  *Resource // クローン元DB
	//ReplicaPassword  string // in current API version , setted admin password
}

// NewCreatePostgreSQLDatabaseValue PostgreSQL作成用パラメーター
func NewCreatePostgreSQLDatabaseValue() *CreateDatabaseValue {
	return &CreateDatabaseValue{
		DatabaseName:    "postgres",
		DatabaseVersion: "9.6",
	}
}

// NewCreateMariaDBDatabaseValue MariaDB作成用パラメーター
func NewCreateMariaDBDatabaseValue() *CreateDatabaseValue {
	return &CreateDatabaseValue{
		DatabaseName:    "MariaDB",
		DatabaseVersion: "10.1",
	}
}

// NewCloneDatabaseValue クローンDB作成用パラメータ
func NewCloneDatabaseValue(db *Database) *CreateDatabaseValue {
	return &CreateDatabaseValue{
		DatabaseName:    db.Remark.DBConf.Common.DatabaseName,
		DatabaseVersion: db.Remark.DBConf.Common.DatabaseVersion,
		SourceAppliance: NewResource(db.ID),
	}
}

// CreateNewDatabase データベース作成
func CreateNewDatabase(values *CreateDatabaseValue) *Database {

	db := &Database{
		// Appliance
		Appliance: &Appliance{
			// Class
			Class: "database",
			// Name
			propName: propName{Name: values.Name},
			// Description
			propDescription: propDescription{Description: values.Description},
			// TagsType
			propTags: propTags{
				// Tags
				Tags: values.Tags,
			},
			// Icon
			propIcon: propIcon{
				&Icon{
					// Resource
					Resource: values.Icon,
				},
			},
			// Plan
			//propPlanID: propPlanID{Plan: &Resource{ID: int64(values.Plan)}},
		},
		// Remark
		Remark: &DatabaseRemark{
			// ApplianceRemarkBase
			ApplianceRemarkBase: &ApplianceRemarkBase{
				// Servers
				Servers: []interface{}{""},
			},
			// DBConf
			DBConf: &DatabaseCommonRemarks{
				// Common
				Common: &DatabaseCommonRemark{
					// DatabaseName
					DatabaseName: values.DatabaseName,
					// DatabaseRevision
					DatabaseRevision: values.DatabaseRevision,
					// DatabaseTitle
					DatabaseTitle: values.DatabaseTitle,
					// DatabaseVersion
					DatabaseVersion: values.DatabaseVersion,
					// ReplicaUser
					// ReplicaUser: values.ReplicaUser,
					// ReplicaPassword
					// ReplicaPassword: values.AdminPassword,
				},
			},
			// Plan
			propPlanID:      propPlanID{Plan: &Resource{ID: int64(values.Plan)}},
			SourceAppliance: values.SourceAppliance,
		},
		// Settings
		Settings: &DatabaseSettings{
			// DBConf
			DBConf: &DatabaseSetting{
				// Backup
				Backup: &DatabaseBackupSetting{
					// Rotate
					// Rotate: values.BackupRotate,
					Rotate: 8,
					// Time
					Time: values.BackupTime,
				},
				// Common
				Common: &DatabaseCommonSetting{
					// DefaultUser
					DefaultUser: values.DefaultUser,
					// UserPassword
					UserPassword: values.UserPassword,
					// SourceNetwork
					SourceNetwork: SourceNetwork(values.SourceNetwork),
					// ServicePort
					ServicePort: values.ServicePort,
				},
			},
		},
	}

	db.Remark.Switch = &ApplianceRemarkSwitch{
		// ID
		ID: values.SwitchID,
	}
	db.Remark.Network = &DatabaseRemarkNetwork{
		// NetworkMaskLen
		NetworkMaskLen: values.MaskLen,
		// DefaultRoute
		DefaultRoute: values.DefaultRoute,
	}

	db.Remark.Servers = []interface{}{
		map[string]interface{}{"IPAddress": values.IPAddress1},
	}

	if values.WebUI {
		db.Settings.DBConf.Common.WebUI = values.WebUI
	}

	return db
}

// CloneNewDatabase データベース作成
func CloneNewDatabase(values *CreateDatabaseValue) *Database {
	db := &Database{
		// Appliance
		Appliance: &Appliance{
			// Class
			Class: "database",
			// Name
			propName: propName{Name: values.Name},
			// Description
			propDescription: propDescription{Description: values.Description},
			// TagsType
			propTags: propTags{
				// Tags
				Tags: values.Tags,
			},
			// Icon
			propIcon: propIcon{
				&Icon{
					// Resource
					Resource: values.Icon,
				},
			},
			// Plan
			//propPlanID: propPlanID{Plan: &Resource{ID: int64(values.Plan)}},
		},
		// Remark
		Remark: &DatabaseRemark{
			// ApplianceRemarkBase
			ApplianceRemarkBase: &ApplianceRemarkBase{
				// Servers
				Servers: []interface{}{""},
			},
			// DBConf
			DBConf: &DatabaseCommonRemarks{
				// Common
				Common: &DatabaseCommonRemark{
					DatabaseName:    values.DatabaseName,
					DatabaseVersion: values.DatabaseVersion,
				},
			},
			// Plan
			propPlanID:      propPlanID{Plan: &Resource{ID: int64(values.Plan)}},
			SourceAppliance: values.SourceAppliance,
		},
		// Settings
		Settings: &DatabaseSettings{
			// DBConf
			DBConf: &DatabaseSetting{
				// Backup
				Backup: &DatabaseBackupSetting{
					// Rotate
					// Rotate: values.BackupRotate,
					Rotate: 8,
					// Time
					Time: values.BackupTime,
				},
				// Common
				Common: &DatabaseCommonSetting{
					// SourceNetwork
					SourceNetwork: SourceNetwork(values.SourceNetwork),
					// ServicePort
					ServicePort: values.ServicePort,
				},
			},
		},
	}

	db.Remark.Switch = &ApplianceRemarkSwitch{
		// ID
		ID: values.SwitchID,
	}
	db.Remark.Network = &DatabaseRemarkNetwork{
		// NetworkMaskLen
		NetworkMaskLen: values.MaskLen,
		// DefaultRoute
		DefaultRoute: values.DefaultRoute,
	}

	db.Remark.Servers = []interface{}{
		map[string]interface{}{"IPAddress": values.IPAddress1},
	}

	if values.WebUI {
		db.Settings.DBConf.Common.WebUI = values.WebUI
	}

	return db
}

// AddSourceNetwork 接続許可ネットワーク 追加
func (s *Database) AddSourceNetwork(nw string) {
	res := []string(s.Settings.DBConf.Common.SourceNetwork)
	res = append(res, nw)
	s.Settings.DBConf.Common.SourceNetwork = SourceNetwork(res)
}

// DeleteSourceNetwork 接続許可ネットワーク 削除
func (s *Database) DeleteSourceNetwork(nw string) {
	res := []string{}
	for _, s := range s.Settings.DBConf.Common.SourceNetwork {
		if s != nw {
			res = append(res, s)
		}
	}
	s.Settings.DBConf.Common.SourceNetwork = SourceNetwork(res)
}
