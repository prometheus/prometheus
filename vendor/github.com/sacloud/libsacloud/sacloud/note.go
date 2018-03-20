package sacloud

// Note スタートアップスクリプト
type Note struct {
	*Resource               // ID
	propName                // 名称
	propDescription         // 説明
	propAvailability        // 有功状態
	propScope               // スコープ
	propIcon                // アイコン
	propTags                // タグ
	propCreatedAt           // 作成日時
	PropModifiedAt          // 変更日時
	propNoteClass           // クラス
	Content          string // スクリプト本体
}
