package sacloud

// propSizeMB サイズ(MB)内包型
type propSizeMB struct {
	SizeMB int `json:",omitempty"` // サイズ(MB単位)
}

// GetSizeMB サイズ(MB単位) 取得
func (p *propSizeMB) GetSizeMB() int {
	return p.SizeMB
}

// SetSizeMB サイズ(MB単位) 設定
func (p *propSizeMB) SetSizeMB(size int) {
	p.SizeMB = size
}

// GetSizeGB サイズ(GB単位) 取得
func (p *propSizeMB) GetSizeGB() int {
	if p.SizeMB <= 0 {
		return 0
	}
	return p.SizeMB / 1024
}

// SetSizeGB サイズ(GB単位) 設定
func (p *propSizeMB) SetSizeGB(size int) {
	if size <= 0 {
		p.SizeMB = 0
	} else {
		p.SizeMB = size * 1024
	}
}

// propMigratedMB コピー済みデータサイズ(MB単位)内包型
type propMigratedMB struct {
	MigratedMB int `json:",omitempty"` // コピー済みデータサイズ(MB単位)
}

// GetMigratedMB サイズ(MB単位) 取得
func (p *propMigratedMB) GetMigratedMB() int {
	return p.MigratedMB
}

// SetMigratedMB サイズ(MB単位) 設定
func (p *propMigratedMB) SetMigratedMB(size int) {
	p.MigratedMB = size
}

// GetMigratedGB サイズ(GB単位) 取得
func (p *propMigratedMB) GetMigratedGB() int {
	if p.MigratedMB <= 0 {
		return 0
	}
	return p.MigratedMB / 1024
}

// SetMigratedGB サイズ(GB単位) 設定
func (p *propMigratedMB) SetMigratedGB(size int) {
	if size <= 0 {
		p.MigratedMB = 0
	} else {
		p.MigratedMB = size * 1024
	}
}
