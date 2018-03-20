package sacloud

// Account さくらのクラウド アカウント
type Account struct {
	*Resource
	propName        // 名称
	Class    string `json:",omitempty"` // リソースクラス
	Code     string `json:",omitempty"` // アカウントコード
}
