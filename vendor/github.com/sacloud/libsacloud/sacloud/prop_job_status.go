package sacloud

// propJobStatus マイグレーションジョブステータス内包型
type propJobStatus struct {
	JobStatus *MigrationJobStatus `json:",omitempty"` // マイグレーションジョブステータス
}

// GetJobStatus マイグレーションジョブステータス 取得
func (p *propJobStatus) GetJobStatus() *MigrationJobStatus {
	return p.JobStatus
}
