package sacloud

// License ライセンス
type License struct {
	*Resource       // ID
	propName        // 名称
	propDescription // 説明
	propCreatedAt   // 作成日時
	PropModifiedAt  // 変更日時

	LicenseInfo *ProductLicense `json:",omitempty"` // ライセンス情報
}

// GetLicenseInfo ライセンス情報 取得
func (l *License) GetLicenseInfo() *ProductLicense {
	return l.LicenseInfo
}

// SetLicenseInfo ライセンス情報 設定
func (l *License) SetLicenseInfo(license *ProductLicense) {
	l.LicenseInfo = license
}

// SetLicenseInfoByID ライセンス情報 設定
func (l *License) SetLicenseInfoByID(id int64) {
	l.LicenseInfo = &ProductLicense{Resource: NewResource(id)}
}
