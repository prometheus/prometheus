package sacloud

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// DatabaseStatus データベース ステータス
type DatabaseStatus struct {
	*EServerInstanceStatus
	DBConf *DatabaseStatusDBConf `json:",omitempty"`
}

// DatabaseStatusDBConf データベース設定
type DatabaseStatusDBConf struct {
	Version *DatabaseStatusVersion `json:"version,omitempty"`
	Log     []*DatabaseLog         `json:"log,omitempty"`
	Backup  *DatabaseBackupInfo    `json:"backup,omitempty"`
}

// DatabaseStatusVersion データベース設定バージョン情報
type DatabaseStatusVersion struct {
	LastModified string `json:"lastmodified,omitempty"`
	CommitHash   string `json:"commithash,omitempty"`
	Status       string `json:"status,omitempty"`
	Tag          string `json:"tag,omitempty"`
	Expire       string `json:"expire,omitempty"`
}

// DatabaseLog データベースログ
type DatabaseLog struct {
	Name string `json:"name,omitempty"`
	Data string `json:"data,omitempty"`
}

// IsSystemdLog systemcltのログか判定
func (l *DatabaseLog) IsSystemdLog() bool {
	return l.Name == "systemctl"
}

// Logs ログボディ取得
func (l *DatabaseLog) Logs() []string {
	return strings.Split(l.Data, "\n")
}

// ID ログのID取得
func (l *DatabaseLog) ID() string {
	return l.Name
}

// DatabaseBackupInfo データベースバックアップ情報
type DatabaseBackupInfo struct {
	History []*DatabaseBackupHistory `json:"history,omitempty"`
}

// DatabaseBackupHistory データベースバックアップ履歴情報
type DatabaseBackupHistory struct {
	CreatedAt    time.Time  `json:"createdat,omitempty"`
	Availability string     `json:"availability,omitempty"`
	RecoveredAt  *time.Time `json:"recoveredat,omitempty"`
	Size         int64      `json:"size,omitempty"`
}

// ID バックアップ履歴のID取得
func (h *DatabaseBackupHistory) ID() string {
	return h.CreatedAt.Format(time.RFC3339)
}

// FormatCreatedAt 指定のレイアウトで作成日時を文字列化
func (h *DatabaseBackupHistory) FormatCreatedAt(layout string) string {
	return h.CreatedAt.Format(layout)
}

// FormatRecoveredAt 指定のレイアウトで復元日時を文字列化
//
// 復元日時がnilの場合は空の文字列を返す
func (h *DatabaseBackupHistory) FormatRecoveredAt(layout string) string {
	if h.RecoveredAt == nil {
		return ""
	}
	return h.RecoveredAt.Format(layout)
}

// UnmarshalJSON JSON復号処理
func (h *DatabaseBackupHistory) UnmarshalJSON(data []byte) error {

	var tmpMap = map[string]interface{}{}
	if err := json.Unmarshal(data, &tmpMap); err != nil {
		return err
	}

	if recoveredAt, ok := tmpMap["recoveredat"]; ok {
		if strRecoveredAt, ok := recoveredAt.(string); ok {
			if _, err := time.Parse(time.RFC3339, strRecoveredAt); err != nil {
				tmpMap["recoveredat"] = nil
			}
		}
	}

	data, err := json.Marshal(tmpMap)
	if err != nil {
		return err
	}

	tmp := &struct {
		CreatedAt    time.Time  `json:"createdat,omitempty"`
		Availability string     `json:"availability,omitempty"`
		RecoveredAt  *time.Time `json:"recoveredat,omitempty"`
		Size         string     `json:"size,omitempty"`
	}{}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	h.CreatedAt = tmp.CreatedAt
	h.Availability = tmp.Availability
	h.RecoveredAt = tmp.RecoveredAt
	s, err := strconv.ParseInt(tmp.Size, 10, 64)
	if err == nil {
		h.Size = s
	} else {
		return err
	}

	return nil
}
