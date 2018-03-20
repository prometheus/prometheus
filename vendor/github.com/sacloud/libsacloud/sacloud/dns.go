package sacloud

import (
	"fmt"
	"strings"
)

// DNS DNS(CommonServiceItem)
type DNS struct {
	*Resource        // ID
	propName         // 名称
	propDescription  // 説明
	propServiceClass // サービスクラス
	propIcon         // アイコン
	propTags         // タグ
	propCreatedAt    // 作成日時
	PropModifiedAt   // 変更日時

	Status   DNSStatus   `json:",omitempty"` // ステータス
	Provider DNSProvider `json:",omitempty"` // プロバイダ
	Settings DNSSettings `json:",omitempty"` // 設定

}

// DNSSettings DNS設定リスト
type DNSSettings struct {
	DNS DNSRecordSets `json:",omitempty"` // DNSレコード設定リスト
}

// DNSStatus DNSステータス
type DNSStatus struct {
	Zone string   `json:",omitempty"` // 対象ゾーン
	NS   []string `json:",omitempty"` // ネームサーバーリスト
}

// DNSProvider プロバイダ
type DNSProvider struct {
	Class string `json:",omitempty"` // クラス
}

// CreateNewDNS DNS作成
func CreateNewDNS(zoneName string) *DNS {
	return &DNS{
		Resource: &Resource{},
		propName: propName{Name: zoneName},
		Status: DNSStatus{
			Zone: zoneName,
		},
		Provider: DNSProvider{
			Class: "dns",
		},
		Settings: DNSSettings{
			DNS: DNSRecordSets{},
		},
	}
}

// AllowDNSTypes DNSレコード種別リスト
func AllowDNSTypes() []string {
	return []string{"A", "AAAA", "CNAME", "NS", "MX", "TXT", "SRV"}
}

// SetZone DNSゾーン名 設定
func (d *DNS) SetZone(zone string) {
	d.Name = zone
	d.Status.Zone = zone
}

// HasDNSRecord DNSレコード設定を保持しているか判定
func (d *DNS) HasDNSRecord() bool {
	return len(d.Settings.DNS.ResourceRecordSets) > 0
}

// CreateNewRecord DNSレコード作成(汎用)
func (d *DNS) CreateNewRecord(name string, rtype string, rdata string, ttl int) *DNSRecordSet {
	return &DNSRecordSet{
		// Name
		Name: name,
		// Type
		Type: rtype,
		// RData
		RData: rdata,
		// TTL
		TTL: ttl,
	}
}

// CreateNewMXRecord DNSレコード作成(MXレコード)
func (d *DNS) CreateNewMXRecord(name string, rdata string, ttl int, priority int) *DNSRecordSet {
	if rdata != "" && !strings.HasSuffix(rdata, ".") {
		rdata = rdata + "."
	}
	return &DNSRecordSet{
		// Name
		Name: name,
		// Type
		Type: "MX",
		// RData
		RData: fmt.Sprintf("%d %s", priority, rdata),
		// TTL
		TTL: ttl,
	}
}

// CreateNewSRVRecord DNSレコード作成(SRVレコード)
func (d *DNS) CreateNewSRVRecord(name string, rdata string, ttl int, priority int, weight int, port int) *DNSRecordSet {
	return &DNSRecordSet{
		// Name
		Name: name,
		// Type
		Type: "SRV",
		// RData
		RData: fmt.Sprintf("%d %d %d %s", priority, weight, port, rdata),
		// TTL
		TTL: ttl,
	}
}

// AddRecord レコードの追加
func (d *DNS) AddRecord(record *DNSRecordSet) {
	var recordSet = d.Settings.DNS.ResourceRecordSets
	var isExist = false
	for i := range recordSet {
		if recordSet[i].Name == record.Name && recordSet[i].Type == record.Type && recordSet[i].RData == record.RData {
			d.Settings.DNS.ResourceRecordSets[i].TTL = record.TTL
			isExist = true
		}
	}

	if !isExist {
		d.Settings.DNS.ResourceRecordSets = append(d.Settings.DNS.ResourceRecordSets, *record)
	}

}

// ClearRecords レコード クリア
func (d *DNS) ClearRecords() {
	d.Settings.DNS = DNSRecordSets{}
}

// DNSRecordSets DNSレコード設定リスト
type DNSRecordSets struct {
	// ResourceRecordSets DNSレコード設定リスト
	ResourceRecordSets []DNSRecordSet
}

// AddDNSRecordSet ホスト名とIPアドレスにてAレコードを登録
func (d *DNSRecordSets) AddDNSRecordSet(name string, ip string) {
	var record DNSRecordSet
	var isExist = false
	for i := range d.ResourceRecordSets {
		if d.ResourceRecordSets[i].Name == name && d.ResourceRecordSets[i].Type == "A" {
			d.ResourceRecordSets[i].RData = ip
			isExist = true
		}
	}

	if !isExist {
		record = DNSRecordSet{
			// Name
			Name: name,
			// Type
			Type: "A",
			// RData
			RData: ip,
		}
		d.ResourceRecordSets = append(d.ResourceRecordSets, record)
	}
}

// DeleteDNSRecordSet ホスト名とIPアドレスにてAレコードを削除する
func (d *DNSRecordSets) DeleteDNSRecordSet(name string, ip string) {
	res := []DNSRecordSet{}
	for i := range d.ResourceRecordSets {
		if d.ResourceRecordSets[i].Name != name || d.ResourceRecordSets[i].Type != "A" || d.ResourceRecordSets[i].RData != ip {
			res = append(res, d.ResourceRecordSets[i])
		}
	}

	d.ResourceRecordSets = res
}

// DNSRecordSet DNSレコード設定
type DNSRecordSet struct {
	Name  string `json:",omitempty"` // ホスト名
	Type  string `json:",omitempty"` // レコードタイプ
	RData string `json:",omitempty"` // レコードデータ
	TTL   int    `json:",omitempty"` // TTL
}
