package sacloud

import "strings"

// BundleInfo バンドル情報
type BundleInfo struct {
	HostClass    string `json:",omitempty"`
	ServiceClass string `json:",omitempty"`
}

// propBundleInfo バンドル情報内包型
type propBundleInfo struct {
	BundleInfo *BundleInfo `json:",omitempty"` // バンドル情報
}

// GetBundleInfo バンドル情報 取得
func (p *propBundleInfo) GetBundleInfo() *BundleInfo {
	return p.BundleInfo
}

func (p *propBundleInfo) IsSophosUTM() bool {
	// SophosUTMであれば編集不可
	if p.BundleInfo != nil && strings.Contains(strings.ToLower(p.BundleInfo.ServiceClass), "sophosutm") {
		return true
	}
	return false
}
