package sacloud

// propOriginalArchiveID オリジナルアーカイブID内包型
type propOriginalArchiveID struct {
	OriginalArchive *Resource `json:",omitempty"` // オリジナルアーカイブ
}

// GetOriginalArchiveID プランID 取得
func (p *propOriginalArchiveID) GetOriginalArchiveID() int64 {
	if p.OriginalArchive == nil {
		return -1
	}
	return p.OriginalArchive.GetID()
}

// GetStrOriginalArchiveID プランID(文字列) 取得
func (p *propOriginalArchiveID) GetStrOriginalArchiveID() string {
	if p.OriginalArchive == nil {
		return ""
	}
	return p.OriginalArchive.GetStrID()
}
