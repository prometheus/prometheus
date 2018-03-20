package sacloud

// SSHKey 公開鍵
type SSHKey struct {
	*Resource       // ID
	propName        // 名称
	propDescription // 説明
	propCreatedAt   // 作成日時

	PublicKey   string `json:",omitempty"` // 公開鍵
	Fingerprint string `json:",omitempty"` // フィンガープリント
}

// SSHKeyGenerated 公開鍵生成戻り値(秘密鍵のダウンロード用)
type SSHKeyGenerated struct {
	SSHKey
	PrivateKey string `json:",omitempty"` // 秘密鍵
}

// GetPublicKey 公開鍵取得
func (k *SSHKey) GetPublicKey() string {
	return k.PublicKey
}

// SetPublicKey 公開鍵設定
func (k *SSHKey) SetPublicKey(pKey string) {
	k.PublicKey = pKey
}

// GetFingerprint フィンガープリント取得
func (k *SSHKey) GetFingerprint() string {
	return k.Fingerprint
}

// GetPrivateKey 秘密鍵取得
func (k *SSHKeyGenerated) GetPrivateKey() string {
	return k.PrivateKey
}
