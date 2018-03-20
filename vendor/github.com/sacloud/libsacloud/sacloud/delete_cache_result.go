package sacloud

// DeleteCacheResult ウェブアクセラレータ キャッシュ削除APIレスポンス
type DeleteCacheResult struct {
	URL    string `json:",omitempty"` // URL
	Status int    `json:",omitempty"` // ステータス
	Result string `json:",omitempty"` // 結果
}
