package sacloud

// CDROM ISOイメージ(CDROM)
type CDROM struct {
	*Resource        // ID
	propName         // 名称
	propDescription  // 説明
	propAvailability // 有功状態
	propSizeMB       // サイズ(MB単位)
	propScope        // スコープ
	propServiceClass // サービスクラス
	propStorageClass // ストレージクラス
	propStorage      // ストレージ
	propIcon         // アイコン
	propTags         // タグ
	propCreatedAt    // 作成日時
}
