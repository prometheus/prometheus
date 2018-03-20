package sacloud

// propCopySource コピー元情報内包型
type propCopySource struct {
	SourceDisk    *Disk    `json:",omitempty"` // コピー元ディスク
	SourceArchive *Archive `json:",omitempty"` // コピー元アーカイブ

}

// SetSourceArchive ソースアーカイブ設定
func (p *propCopySource) SetSourceArchive(sourceID int64) {
	if sourceID == EmptyID {
		return
	}
	p.SourceArchive = &Archive{
		Resource: &Resource{ID: sourceID},
	}
	p.SourceDisk = nil
}

// SetSourceDisk ソースディスク設定
func (p *propCopySource) SetSourceDisk(sourceID int64) {
	if sourceID == EmptyID {
		return
	}
	p.SourceDisk = &Disk{
		Resource: &Resource{ID: sourceID},
	}
	p.SourceArchive = nil
}

// GetSourceArchive ソースアーカイブ取得
func (p *propCopySource) GetSourceArchive() *Archive {
	return p.SourceArchive
}

// GetSourceDisk ソースディスク取得
func (p *propCopySource) GetSourceDisk() *Disk {
	return p.SourceDisk
}

// GetSourceArchiveID ソースアーカイブID取得
func (p *propCopySource) GetSourceArchiveID() int64 {
	if p.SourceArchive != nil {
		return p.SourceArchive.GetID()
	}
	return EmptyID
}

// GetSourceDiskID ソースディスクID取得
func (p *propCopySource) GetSourceDiskID() int64 {
	if p.SourceDisk != nil {
		return p.SourceDisk.GetID()
	}
	return EmptyID
}
