package cdn

type DomainConfigRequest struct {
	DomainName string
	// optional
	ConfigList string
}

type DomainConfigResponse struct {
	CdnCommonResponse
	DomainConfigs DomainConfigs
}

type DomainConfigs struct {
	PageCompressConfig struct {
		Enable   string
		ConfigId string
		Status   string
	}
	WafConfig struct {
		Enable string
	}
	IgnoreQueryStringConfig struct {
		Enable      string
		ConfigId    string
		Status      string
		HashKeyArgs string
	}
	VideoSeekConfig struct {
		Enable   string
		ConfigId string
		Status   string
	}
	ReqAuthConfig struct {
		Key1     string
		Key2     string
		AuthType string
		TimeOut  string
	}
	RangeConfig struct {
		Enable   string
		ConfigId string
		Status   string
	}
	SrcHostConfig struct {
		DomainName string
	}
	OptimizeConfig struct {
		Enable   string
		ConfigId string
		Status   string
	}
	ErrorPageConfig struct {
		PageType      string
		CustomPageUrl string
		ErrorCode     string
	}
	RefererConfig struct {
		AllowEmpty string
		ReferType  string
		ReferList  string
	}
	CcConfig struct {
		Enable   string
		AllowIps string
		BlockIps string
		ConfigId string
		Status   string
	}
	HttpHeaderConfigs struct {
		HttpHeaderConfig []HttpHeaderConfig
	}
	CacheExpiredConfigs struct {
		CacheExpiredConfig []CacheExpiredConfig
	}
	NotifyUrlConfig struct {
		Enable    string
		NotifyUrl string
	}
	RedirectTypeConfig struct {
		RedirectType string
	}
}

type HttpHeaderConfig struct {
	Status      string
	HeaderKey   string
	HeaderValue string
	ConfigId    string
}

type CacheExpiredConfig struct {
	Status       string
	TTL          string
	CacheType    string
	Weight       string
	CacheContent string
	ConfigId     string
}

type ConfigRequest struct {
	DomainName string
	Enable     string
}

type QueryStringConfigRequest struct {
	DomainName string
	Enable     string
	// optional
	HashKeyArgs string
}

type HostConfigRequest struct {
	DomainName    string
	BackSrcDomain string
}

type ErrorPageConfigRequest struct {
	DomainName string
	PageType   string
	// optional
	CustomPageUrl string
}

type RedirectConfigRequest struct {
	DomainName   string
	RedirectType string
}

type ReferConfigRequest struct {
	DomainName string
	ReferType  string
	// optional
	ReferList  string
	AllowEmpty string
}

type CacheConfigRequest struct {
	DomainName   string
	CacheContent string
	TTL          string
	// optional
	Weight string
}

type ModifyCacheConfigRequest struct {
	CacheConfigRequest
	ConfigID string
}

type DeleteCacheConfigRequest struct {
	DomainName string
	CacheType  string
	ConfigID   string
}

type ReqAuthConfigRequest struct {
	DomainName string
	AuthType   string
	// optional
	Key1    string
	Key2    string
	Timeout string
}

type HttpHeaderConfigRequest struct {
	DomainName  string
	HeaderKey   string
	HeaderValue string
}

type ModifyHttpHeaderConfigRequest struct {
	HttpHeaderConfigRequest
	ConfigID string
}

type DeleteHttpHeaderConfigRequest struct {
	DomainName string
	ConfigID   string
}

type CertificateRequest struct {
	DomainName              string
	CertName                string
	ServerCertificateStatus string
	// optional
	ServerCertificate string
	PrivateKey        string
}

type IpBlackRequest struct {
	DomainName string
	// optional
	Enable   string
	BlockIps string
}

func (client *CdnClient) DescribeDomainConfigs(req DomainConfigRequest) (DomainConfigResponse, error) {
	var resp DomainConfigResponse
	err := client.Invoke("DescribeDomainConfigs", req, &resp)
	if err != nil {
		return DomainConfigResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) SetOptimizeConfig(req ConfigRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("SetOptimizeConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) SetPageCompressConfig(req ConfigRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("SetPageCompressConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) SetIgnoreQueryStringConfig(req QueryStringConfigRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("SetIgnoreQueryStringConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) SetRangeConfig(req ConfigRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("SetRangeConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) SetVideoSeekConfig(req ConfigRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("SetVideoSeekConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) SetSourceHostConfig(req HostConfigRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("SetSourceHostConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) SetErrorPageConfig(req ErrorPageConfigRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("SetErrorPageConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) SetForceRedirectConfig(req RedirectConfigRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("SetForceRedirectConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) SetRefererConfig(req ReferConfigRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("SetRefererConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) SetFileCacheExpiredConfig(req CacheConfigRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("SetFileCacheExpiredConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) SetPathCacheExpiredConfig(req CacheConfigRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("SetPathCacheExpiredConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) ModifyFileCacheExpiredConfig(req ModifyCacheConfigRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("ModifyFileCacheExpiredConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) ModifyPathCacheExpiredConfig(req ModifyCacheConfigRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("ModifyPathCacheExpiredConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) DeleteCacheExpiredConfig(req DeleteCacheConfigRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("DeleteCacheExpiredConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) SetReqAuthConfig(req ReqAuthConfigRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("SetReqAuthConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) SetHttpHeaderConfig(req HttpHeaderConfigRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("SetHttpHeaderConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) ModifyHttpHeaderConfig(req ModifyHttpHeaderConfigRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("ModifyHttpHeaderConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) DeleteHttpHeaderConfig(req DeleteHttpHeaderConfigRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("DeleteHttpHeaderConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) SetDomainServerCertificate(req CertificateRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("SetDomainServerCertificate", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) SetIpBlackListConfig(req IpBlackRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("SetCcConfig", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}
