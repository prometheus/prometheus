package sacloud

// propDisks ディスク配列内包型
type propDisks struct {
	Disks []Disk `json:",omitempty"` // ディスク
}

// GetDisks ディスク配列 取得
func (p *propDisks) GetDisks() []Disk {
	return p.Disks
}

// GetDiskIDs ディスクID配列を返す
func (p *propDisks) GetDiskIDs() []int64 {

	ids := []int64{}
	for _, disk := range p.Disks {
		ids = append(ids, disk.ID)
	}
	return ids

}
