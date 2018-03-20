package sacloud

import "fmt"

// Disk ディスク
type Disk struct {
	*Resource          // ID
	propAvailability   // 有功状態
	propName           // 名称
	propDescription    // 説明
	propSizeMB         // サイズ(MB単位)
	propMigratedMB     // コピー済みデータサイズ(MB単位)
	propCopySource     // コピー元情報
	propJobStatus      // マイグレーションジョブステータス
	propBundleInfo     // バンドル情報
	propServer         // サーバー
	propIcon           // アイコン
	propTags           // タグ
	propCreatedAt      // 作成日時
	propPlanID         // プランID
	propDiskConnection // ディスク接続情報
	propDistantFrom    // ストレージ隔離対象ディスク

	ReinstallCount int `json:",omitempty"` // 再インストール回数

	Storage struct { // ストレージ
		*Resource              // ID
		Name       string      `json:",omitempty"`  // 名称
		DiskPlan   ProductDisk `json:",ommitempty"` // ディスクプラン
		MountIndex int64       `json:",omitempty"`  // マウント順
		Class      string      `json:",omitempty"`  // クラス
	}
}

// DiskPlanID ディスクプランID
type DiskPlanID int64

const (
	// DiskPlanHDDID HDDプランID
	DiskPlanHDDID = DiskPlanID(2)
	// DiskPlanSSDID SSDプランID
	DiskPlanSSDID = DiskPlanID(4)
	// DiskConnectionVirtio 準仮想化モード(virtio)
	DiskConnectionVirtio EDiskConnection = "virtio"
	// DiskConnectionIDE IDE
	DiskConnectionIDE EDiskConnection = "ide"
)

var (
	// DiskPlanHDD HDDプラン
	DiskPlanHDD = &Resource{ID: int64(DiskPlanHDDID)}
	// DiskPlanSSD SSDプラン
	DiskPlanSSD = &Resource{ID: int64(DiskPlanSSDID)}
)

// ToResource ディスクプランIDからリソースへの変換
func (d DiskPlanID) ToResource() *Resource {
	return &Resource{ID: int64(d)}
}

// CreateNewDisk ディスクの作成
func CreateNewDisk() *Disk {
	return &Disk{
		propPlanID:         propPlanID{Plan: DiskPlanSSD},
		propDiskConnection: propDiskConnection{Connection: DiskConnectionVirtio},
		propSizeMB:         propSizeMB{SizeMB: 20480},
	}
}

// SetDiskPlan プラン文字列(ssd or sdd)からプラン設定
func (d *Disk) SetDiskPlan(strPlan string) {
	switch strPlan {
	case "ssd":
		d.Plan = DiskPlanSSD
	case "hdd":
		d.Plan = DiskPlanHDD
	default:
		panic(fmt.Errorf("Invalid plan:%s", strPlan))
	}
}

// SetDiskPlanToHDD HDDプラン 設定
func (d *Disk) SetDiskPlanToHDD() {
	d.Plan = DiskPlanHDD
}

// SetDiskPlanToSSD SSDプラン 設定
func (d *Disk) SetDiskPlanToSSD() {
	d.Plan = DiskPlanSSD
}

// DiskEditValue ディスクの修正用パラメータ
//
// 設定を行う項目のみ値をセットする。値のセットにはセッターを利用すること。
type DiskEditValue struct {
	Password      *string     `json:",omitempty"` // パスワード
	SSHKey        *SSHKey     `json:",omitempty"` // 公開鍵(単体)
	SSHKeys       []*SSHKey   `json:",omitempty"` // 公開鍵(複数)
	DisablePWAuth *bool       `json:",omitempty"` // パスワード認証無効化フラグ
	HostName      *string     `json:",omitempty"` // ホスト名
	Notes         []*Resource `json:",omitempty"` // スタートアップスクリプト
	UserIPAddress *string     `json:",omitempty"` // IPアドレス
	UserSubnet    *struct {   // サブネット情報
		DefaultRoute   string `json:",omitempty"` // デフォルトルート
		NetworkMaskLen string `json:",omitempty"` // ネットワークマスク長
	} `json:",omitempty"`
}

// SetHostName ホスト名 設定
func (d *DiskEditValue) SetHostName(value string) {
	d.HostName = &value
}

// SetPassword パスワード 設定
func (d *DiskEditValue) SetPassword(value string) {
	d.Password = &value
}

// AddSSHKeys 公開鍵 設定
func (d *DiskEditValue) AddSSHKeys(keyID string) {
	if d.SSHKeys == nil {
		d.SSHKeys = []*SSHKey{}
	}
	d.SSHKeys = append(d.SSHKeys, &SSHKey{Resource: NewResourceByStringID(keyID)})
}

// SetSSHKeys 公開鍵 設定
func (d *DiskEditValue) SetSSHKeys(keyIDs []string) {
	if d.SSHKeys == nil {
		d.SSHKeys = []*SSHKey{}
	}
	for _, keyID := range keyIDs {
		d.SSHKeys = append(d.SSHKeys, &SSHKey{Resource: NewResourceByStringID(keyID)})
	}
}

// AddSSHKeyByString 公開鍵(文字列) 追加
func (d *DiskEditValue) AddSSHKeyByString(key string) {
	if d.SSHKeys == nil {
		d.SSHKeys = []*SSHKey{}
	}
	d.SSHKeys = append(d.SSHKeys, &SSHKey{PublicKey: key})
}

// SetSSHKeyByString 公開鍵(文字列) 設定
func (d *DiskEditValue) SetSSHKeyByString(keys []string) {
	if d.SSHKeys == nil {
		d.SSHKeys = []*SSHKey{}
	}
	for _, key := range keys {
		d.SSHKeys = append(d.SSHKeys, &SSHKey{PublicKey: key})
	}
}

// SetDisablePWAuth パスワード認証無効化フラグ 設定
func (d *DiskEditValue) SetDisablePWAuth(disable bool) {
	d.DisablePWAuth = &disable
}

// SetNotes スタートアップスクリプト 設定
func (d *DiskEditValue) SetNotes(noteIDs []string) {
	d.Notes = []*Resource{}
	for _, noteID := range noteIDs {
		d.Notes = append(d.Notes, NewResourceByStringID(noteID))
	}

}

// AddNote スタートアップスクリプト 追加
func (d *DiskEditValue) AddNote(noteID string) {
	if d.Notes == nil {
		d.Notes = []*Resource{}
	}
	d.Notes = append(d.Notes, NewResourceByStringID(noteID))
}

// SetUserIPAddress IPアドレス 設定
func (d *DiskEditValue) SetUserIPAddress(ip string) {
	d.UserIPAddress = &ip
}

// SetDefaultRoute デフォルトルート 設定
func (d *DiskEditValue) SetDefaultRoute(route string) {
	if d.UserSubnet == nil {
		d.UserSubnet = &struct {
			DefaultRoute   string `json:",omitempty"`
			NetworkMaskLen string `json:",omitempty"`
		}{}
	}
	d.UserSubnet.DefaultRoute = route
}

// SetNetworkMaskLen ネットワークマスク長 設定
func (d *DiskEditValue) SetNetworkMaskLen(length string) {
	if d.UserSubnet == nil {
		d.UserSubnet = &struct {
			DefaultRoute   string `json:",omitempty"`
			NetworkMaskLen string `json:",omitempty"`
		}{}
	}
	d.UserSubnet.NetworkMaskLen = length
}
