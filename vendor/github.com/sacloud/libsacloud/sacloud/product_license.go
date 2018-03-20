package sacloud

// ProductLicense ライセンスプラン
type ProductLicense struct {
	*Resource               // ID
	propName                // 名称
	propServiceClass        // サービスクラス
	propCreatedAt           // 作成日時
	PropModifiedAt          // 変更日時
	TermsOfUse       string `json:",omitempty"` // 利用規約
}
