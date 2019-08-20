package cdn

import "github.com/denverdino/aliyungo/common"

type AddDomainRequest struct {
	DomainName string
	CdnType    string
	//optional
	SourceType string
	SourcePort int
	Sources    string
	Scope      string
}

type DescribeDomainsRequest struct {
	//optional
	common.Pagination
	DomainName       string
	DomainStatus     string
	DomainSearchType string
}

type DomainsResponse struct {
	CdnCommonResponse
	common.PaginationResult
	Domains struct {
		PageData []Domains
	}
}

type DescribeDomainRequest struct {
	DomainName string
}

type DomainResponse struct {
	CdnCommonResponse
	GetDomainDetailModel DomainDetail
}

type DomainDetail struct {
	DomainName              string
	Cname                   string
	HttpsCname              string
	Scope                   string
	CdnType                 string
	SourceType              string
	GmtCreated              string
	GmtModified             string
	DomainStatus            string
	CertificateName         string
	ServerCertificate       string
	ServerCertificateStatus string
	Sources                 struct {
		Source []string
	}
}

type ModifyDomainRequest struct {
	DomainName string
	SourceType string
	SourcePort int
	Sources    string
}

type DescribeDomainsBySourceRequest struct {
	Sources string
}

type DomainInfo struct {
	DomainName  string
	Status      string
	CreateTime  string
	UpdateTime  string
	DomainCname string
}

type DomainsData struct {
	Source  string
	Domains struct {
		domainNames []string
	}
	DomainInfos struct {
		domainInfo []DomainInfo
	}
}

type DomainBySourceResponse struct {
	CdnCommonResponse
	Sources     string
	DomainsList struct {
		DomainsData []DomainsData
	}
}

func (client *CdnClient) AddCdnDomain(req AddDomainRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("AddCdnDomain", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) DescribeUserDomains(req DescribeDomainsRequest) (DomainsResponse, error) {
	var resp DomainsResponse
	err := client.Invoke("DescribeUserDomains", req, &resp)
	if err != nil {
		return DomainsResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) DescribeCdnDomainDetail(req DescribeDomainRequest) (DomainResponse, error) {
	var resp DomainResponse
	err := client.Invoke("DescribeCdnDomainDetail", req, &resp)
	if err != nil {
		return DomainResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) ModifyCdnDomain(req ModifyDomainRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("ModifyCdnDomain", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) StartCdnDomain(req DescribeDomainRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("StartCdnDomain", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) StopCdnDomain(req DescribeDomainRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("StopCdnDomain", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) DeleteCdnDomain(req DescribeDomainRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("DeleteCdnDomain", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) DescribeDomainsBySource(req DescribeDomainsBySourceRequest) (DomainBySourceResponse, error) {
	var resp DomainBySourceResponse
	err := client.Invoke("DescribeDomainsBySource", req, &resp)
	if err != nil {
		return DomainBySourceResponse{}, err
	}
	return resp, nil
}
