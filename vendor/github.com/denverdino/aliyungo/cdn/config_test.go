package cdn

import (
	"testing"
)

var (
	dName           = "www.jb51.net"
	headerConfigReq = HttpHeaderConfigRequest{
		DomainName:  dName,
		HeaderKey:   ContentType,
		HeaderValue: "text/plain",
	}
	headerConfigId    = ""
	cacheFileConfigId = ""
	cachePathConfigId = ""
	modifyHeaderReq   = ModifyHttpHeaderConfigRequest{
		HttpHeaderConfigRequest: HttpHeaderConfigRequest{
			DomainName:  dName,
			HeaderKey:   ContentType,
			HeaderValue: "application/json",
		},
	}
	delHeaderReq = DeleteHttpHeaderConfigRequest{
		DomainName: dName,
	}

	descDomainConfigsReq = DomainConfigRequest{
		DomainName: dName,
		ConfigList: "http_header,cache_expired",
	}

	cacheReq = CacheConfigRequest{
		DomainName: dName,
		TTL:        "1000",
	}
	modifyCacheReq = ModifyCacheConfigRequest{
		CacheConfigRequest: cacheReq,
	}
	delCacheReq = DeleteCacheConfigRequest{
		DomainName: dName,
	}
)

func TestSetHttpHeaderConfig(t *testing.T) {
	client := NewTestClient()
	resp, err := client.SetHttpHeaderConfig(headerConfigReq)
	if err != nil {
		t.Errorf("Failed to SetHttpHeaderConfig %v", err)
	}
	t.Logf("pass SetHttpHeaderConfig %v", resp)
}

func TestModifyHttpHeaderConfig(t *testing.T) {
	client := NewTestClient()
	configsResp, _ := client.DescribeDomainConfigs(descDomainConfigsReq)
	for _, conf := range configsResp.DomainConfigs.HttpHeaderConfigs.HttpHeaderConfig {
		if conf.HeaderKey == headerConfigReq.HeaderKey && conf.HeaderValue == headerConfigReq.HeaderValue {
			headerConfigId = conf.ConfigId
			break
		}
	}
	modifyHeaderReq.ConfigID = headerConfigId
	resp, err := client.ModifyHttpHeaderConfig(modifyHeaderReq)
	if err != nil {
		t.Errorf("Failed to ModifyHttpHeaderConfig %v", err)
	}
	t.Logf("pass ModifyHttpHeaderConfig %v", resp)
}

func TestDeleteHttpHeaderConfig(t *testing.T) {
	client := NewTestClient()
	delHeaderReq.ConfigID = headerConfigId
	resp, err := client.DeleteHttpHeaderConfig(delHeaderReq)
	if err != nil {
		t.Errorf("Failed to DeleteHttpHeaderConfig %v", err)
	}
	t.Logf("pass DeleteHttpHeaderConfig %v", resp)
}

func TestSetFileCacheExpiredConfig(t *testing.T) {
	client := NewTestClient()
	cacheReq.CacheContent = "txt,jpg,png"
	resp, err := client.SetFileCacheExpiredConfig(cacheReq)
	if err != nil {
		t.Errorf("Failed to SetFileCacheExpiredConfig %v", err)
	}
	configsResp, _ := client.DescribeDomainConfigs(descDomainConfigsReq)
	for _, conf := range configsResp.DomainConfigs.CacheExpiredConfigs.CacheExpiredConfig {
		if conf.CacheContent == cacheReq.CacheContent && conf.TTL == cacheReq.TTL && conf.CacheType == "suffix" {
			cacheFileConfigId = conf.ConfigId
			break
		}
	}
	t.Logf("pass SetFileCacheExpiredConfig %v", resp)
}

func TestSetPathCacheExpiredConfig(t *testing.T) {
	client := NewTestClient()
	cacheReq.CacheContent = "/hello/world"
	resp, err := client.SetPathCacheExpiredConfig(cacheReq)
	if err != nil {
		t.Errorf("Failed to SetPathCacheExpiredConfig %v", err)
	}
	configsResp, _ := client.DescribeDomainConfigs(descDomainConfigsReq)
	for _, conf := range configsResp.DomainConfigs.CacheExpiredConfigs.CacheExpiredConfig {
		if conf.CacheContent == cacheReq.CacheContent && conf.TTL == cacheReq.TTL && conf.CacheType == "path" {
			cachePathConfigId = conf.ConfigId
			break
		}
	}
	t.Logf("pass SetPathCacheExpiredConfig %v", resp)
}

func TestModifyFileCacheExpiredConfig(t *testing.T) {
	client := NewTestClient()
	modifyCacheReq.CacheConfigRequest.CacheContent = "py,json"
	modifyCacheReq.ConfigID = cacheFileConfigId
	resp, err := client.ModifyFileCacheExpiredConfig(modifyCacheReq)
	if err != nil {
		t.Errorf("Failed to ModifyFileCacheExpiredConfig %v", err)
	}
	t.Logf("pass ModifyFileCacheExpiredConfig %v", resp)
}

func TestModifyPathCacheExpiredConfig(t *testing.T) {
	client := NewTestClient()
	modifyCacheReq.CacheConfigRequest.CacheContent = "/hello/you"
	modifyCacheReq.ConfigID = cachePathConfigId
	resp, err := client.ModifyPathCacheExpiredConfig(modifyCacheReq)
	if err != nil {
		t.Errorf("Failed to ModifyPathCacheExpiredConfig %v", err)
	}
	t.Logf("pass ModifyPathCacheExpiredConfig %v", resp)
}

func TestDeleteCacheExpiredConfig(t *testing.T) {
	client := NewTestClient()
	delCacheReq.ConfigID = cacheFileConfigId
	delCacheReq.CacheType = "suffix"
	resp, err := client.DeleteCacheExpiredConfig(delCacheReq)
	if err != nil {
		t.Errorf("Failed to DeleteCacheExpiredConfig %v", err)
	}

	delCacheReq.ConfigID = cachePathConfigId
	delCacheReq.CacheType = "path"
	resp, err = client.DeleteCacheExpiredConfig(delCacheReq)
	if err != nil {
		t.Errorf("Failed to DeleteCacheExpiredConfig %v", err)
	}
	t.Logf("pass DeleteCacheExpiredConfig %v", resp)
}
