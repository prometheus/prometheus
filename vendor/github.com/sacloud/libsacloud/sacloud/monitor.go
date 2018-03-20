package sacloud

import (
	"math"
	"time"
)

// MonitorValue アクティビティモニター
type MonitorValue struct {
	CPUTime         *float64 `json:"CPU-TIME,omitempty"`          // CPU時間
	Write           *float64 `json:",omitempty"`                  // ディスク書き込み
	Read            *float64 `json:",omitempty"`                  // ディスク読み取り
	Receive         *float64 `json:",omitempty"`                  // パケット受信
	Send            *float64 `json:",omitempty"`                  // パケット送信
	In              *float64 `json:",omitempty"`                  // パケット受信
	Out             *float64 `json:",omitempty"`                  // パケット送信
	TotalMemorySize *float64 `json:"Total-Memory-Size,omitempty"` // 総メモリサイズ
	UsedMemorySize  *float64 `json:"Used-Memory-Size,omitempty"`  // 使用済みメモリサイズ
	TotalDisk1Size  *float64 `json:"Total-Disk1-Size,omitempty"`  // 総ディスクサイズ
	UsedDisk1Size   *float64 `json:"Used-Disk1-Size,omitempty"`   // 使用済みディスクサイズ
	TotalDisk2Size  *float64 `json:"Total-Disk2-Size,omitempty"`  // 総ディスクサイズ
	UsedDisk2Size   *float64 `json:"Used-Disk2-Size,omitempty"`   // 使用済みディスクサイズ
	FreeDiskSize    *float64 `json:"Free-Disk-Size,omitempty"`    // 空きディスクサイズ(NFS)
	ResponseTimeSec *float64 `json:"responsetimesec,omitempty"`   // レスポンスタイム(シンプル監視)
}

// ResourceMonitorRequest アクティビティモニター取得リクエスト
type ResourceMonitorRequest struct {
	Start *time.Time `json:",omitempty"` // 取得開始時間
	End   *time.Time `json:",omitempty"` // 取得終了時間
}

// NewResourceMonitorRequest アクティビティモニター取得リクエスト作成
func NewResourceMonitorRequest(start *time.Time, end *time.Time) *ResourceMonitorRequest {
	res := &ResourceMonitorRequest{}
	if start != nil {
		t := start.Truncate(time.Second)
		res.Start = &t
	}
	if end != nil {
		t := end.Truncate(time.Second)
		res.End = &t
	}
	return res
}

// ResourceMonitorResponse アクティビティモニターレスポンス
type ResourceMonitorResponse struct {
	Data *MonitorValues `json:",omitempty"` // メトリクス
}

// MonitorSummaryData メトリクスサマリー
type MonitorSummaryData struct {
	Max   float64 // 最大値
	Min   float64 // 最小値
	Avg   float64 // 平均値
	Count float64 // データ個数

}

// MonitorSummary アクティビティーモニター サマリー
type MonitorSummary struct {
	CPU  *MonitorSummaryData // CPU時間サマリー
	Disk *struct {           // ディスク利用サマリー
		Write *MonitorSummaryData // ディスク書き込みサマリー
		Read  *MonitorSummaryData // ディスク読み取りサマリー
	}
	Interface *struct { // NIC送受信サマリー
		Receive *MonitorSummaryData // 受信パケットサマリー
		Send    *MonitorSummaryData // 送信パケットサマリー
	}
}

// MonitorValues メトリクス リスト
type MonitorValues map[string]*MonitorValue

// FlatMonitorValue フラット化したメトリクス
type FlatMonitorValue struct {
	Time  time.Time // 対象時刻
	Value float64   // 値
}

// Calc サマリー計算
func (m *MonitorValues) Calc() *MonitorSummary {

	res := &MonitorSummary{}
	res.CPU = m.calcBy(func(v *MonitorValue) *float64 { return v.CPUTime })
	res.Disk = &struct {
		Write *MonitorSummaryData
		Read  *MonitorSummaryData
	}{
		Write: m.calcBy(func(v *MonitorValue) *float64 { return v.Write }),
		Read:  m.calcBy(func(v *MonitorValue) *float64 { return v.Read }),
	}
	res.Interface = &struct {
		Receive *MonitorSummaryData
		Send    *MonitorSummaryData
	}{
		Receive: m.calcBy(func(v *MonitorValue) *float64 { return v.Receive }),
		Send:    m.calcBy(func(v *MonitorValue) *float64 { return v.Send }),
	}

	return res
}

func (m *MonitorValues) calcBy(f func(m *MonitorValue) *float64) *MonitorSummaryData {
	res := &MonitorSummaryData{}
	var sum float64
	for _, data := range map[string]*MonitorValue(*m) {
		value := f(data)
		if value != nil {
			res.Count++
			res.Min = math.Min(res.Min, *value)
			res.Max = math.Max(res.Max, *value)
			sum += *value
		}
	}
	if sum > 0 && res.Count > 0 {
		res.Avg = sum / res.Count
	}

	return res
}

// FlattenCPUTimeValue フラット化 CPU時間
func (m *MonitorValues) FlattenCPUTimeValue() ([]FlatMonitorValue, error) {
	return m.flattenValue(func(v *MonitorValue) *float64 { return v.CPUTime })
}

// FlattenDiskWriteValue フラット化 ディスク書き込み
func (m *MonitorValues) FlattenDiskWriteValue() ([]FlatMonitorValue, error) {
	return m.flattenValue(func(v *MonitorValue) *float64 { return v.Write })
}

// FlattenDiskReadValue フラット化 ディスク読み取り
func (m *MonitorValues) FlattenDiskReadValue() ([]FlatMonitorValue, error) {
	return m.flattenValue(func(v *MonitorValue) *float64 { return v.Read })
}

// FlattenPacketSendValue フラット化 パケット送信
func (m *MonitorValues) FlattenPacketSendValue() ([]FlatMonitorValue, error) {
	return m.flattenValue(func(v *MonitorValue) *float64 { return v.Send })
}

// FlattenPacketReceiveValue フラット化 パケット受信
func (m *MonitorValues) FlattenPacketReceiveValue() ([]FlatMonitorValue, error) {
	return m.flattenValue(func(v *MonitorValue) *float64 { return v.Receive })
}

// FlattenInternetInValue フラット化 パケット受信
func (m *MonitorValues) FlattenInternetInValue() ([]FlatMonitorValue, error) {
	return m.flattenValue(func(v *MonitorValue) *float64 { return v.In })
}

// FlattenInternetOutValue フラット化 パケット送信
func (m *MonitorValues) FlattenInternetOutValue() ([]FlatMonitorValue, error) {
	return m.flattenValue(func(v *MonitorValue) *float64 { return v.Out })
}

// FlattenTotalMemorySizeValue フラット化 総メモリサイズ
func (m *MonitorValues) FlattenTotalMemorySizeValue() ([]FlatMonitorValue, error) {
	return m.flattenValue(func(v *MonitorValue) *float64 { return v.TotalMemorySize })
}

// FlattenUsedMemorySizeValue フラット化 使用済みメモリサイズ
func (m *MonitorValues) FlattenUsedMemorySizeValue() ([]FlatMonitorValue, error) {
	return m.flattenValue(func(v *MonitorValue) *float64 { return v.UsedMemorySize })
}

// FlattenTotalDisk1SizeValue フラット化 総ディスクサイズ
func (m *MonitorValues) FlattenTotalDisk1SizeValue() ([]FlatMonitorValue, error) {
	return m.flattenValue(func(v *MonitorValue) *float64 { return v.TotalDisk1Size })
}

// FlattenUsedDisk1SizeValue フラット化 使用済みディスクサイズ
func (m *MonitorValues) FlattenUsedDisk1SizeValue() ([]FlatMonitorValue, error) {
	return m.flattenValue(func(v *MonitorValue) *float64 { return v.UsedDisk1Size })
}

// FlattenTotalDisk2SizeValue フラット化 総ディスクサイズ
func (m *MonitorValues) FlattenTotalDisk2SizeValue() ([]FlatMonitorValue, error) {
	return m.flattenValue(func(v *MonitorValue) *float64 { return v.TotalDisk2Size })
}

// FlattenUsedDisk2SizeValue フラット化 使用済みディスクサイズ
func (m *MonitorValues) FlattenUsedDisk2SizeValue() ([]FlatMonitorValue, error) {
	return m.flattenValue(func(v *MonitorValue) *float64 { return v.UsedDisk2Size })
}

// FlattenFreeDiskSizeValue フラット化 空きディスクサイズ(NFS)
func (m *MonitorValues) FlattenFreeDiskSizeValue() ([]FlatMonitorValue, error) {
	return m.flattenValue(func(v *MonitorValue) *float64 { return v.FreeDiskSize })
}

// FlattenResponseTimeSecValue フラット化 レスポンスタイム(シンプル監視)
func (m *MonitorValues) FlattenResponseTimeSecValue() ([]FlatMonitorValue, error) {
	return m.flattenValue(func(v *MonitorValue) *float64 { return v.ResponseTimeSec })
}

func (m *MonitorValues) flattenValue(f func(*MonitorValue) *float64) ([]FlatMonitorValue, error) {
	var res []FlatMonitorValue

	for k, v := range map[string]*MonitorValue(*m) {
		if f(v) == nil {
			continue
		}
		time, err := time.Parse(time.RFC3339, k) // RFC3339 ≒ ISO8601
		if err != nil {
			return res, err
		}
		res = append(res, FlatMonitorValue{
			// Time
			Time: time,
			// Value
			Value: *f(v),
		})
	}
	return res, nil
}

// HasValue 取得したアクティビティーモニターに有効値が含まれるか判定
func (m *MonitorValue) HasValue() bool {
	values := []*float64{
		m.CPUTime,
		m.Read, m.Receive,
		m.Send, m.Write,
		m.In, m.Out,
		m.TotalMemorySize, m.UsedMemorySize,
		m.TotalDisk1Size, m.UsedDisk1Size,
		m.TotalDisk2Size, m.UsedDisk2Size,
		m.FreeDiskSize, m.ResponseTimeSec,
	}
	for _, v := range values {
		if v != nil {
			return true
		}
	}
	return false
}
