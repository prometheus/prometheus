package sacloud

// Archive アーカイブ
type Archive struct {
	*Resource             // ID
	propAvailability      // 有功状態
	propName              // 名称
	propDescription       // 説明
	propSizeMB            // サイズ(MB単位)
	propMigratedMB        // コピー済みデータサイズ(MB単位)
	propScope             // スコープ
	propCopySource        // コピー元情報
	propServiceClass      // サービスクラス
	propPlanID            // プランID
	propJobStatus         // マイグレーションジョブステータス
	propOriginalArchiveID // オリジナルアーカイブID
	propStorage           // ストレージ
	propBundleInfo        // バンドル情報
	propTags              // タグ
	propIcon              // アイコン
	propCreatedAt         // 作成日時
}
