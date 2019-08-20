package cdn

import (
	"testing"
)

var (
	domainName = "www.jb51.net"
	domain     = AddDomainRequest{
		DomainName: domainName,
		CdnType:    "web",
		SourceType: "domain",
		Sources:    "aliyun.com",
	}
	descDomainReq = DescribeDomainRequest{
		DomainName: domainName,
	}
	modifyDomainReq = ModifyDomainRequest{
		DomainName: domainName,
		SourceType: "domain",
		SourcePort: 443,
		Sources:    "aliyun.com",
	}
)

func TestAddCdnDomain(t *testing.T) {
	client := NewTestClient()
	resp, err := client.AddCdnDomain(domain)
	if err != nil {
		t.Errorf("Failed to AddCdnDomain %v", err)
	}
	t.Logf("pass AddCdnDomain %v", resp)
}

func TestDescribeCdnDomainDetail(t *testing.T) {
	client := NewTestClient()
	resp, err := client.DescribeCdnDomainDetail(descDomainReq)
	if err != nil {
		t.Errorf("Failed to DescribeCdnDomainDetail %v", err)
	}
	t.Logf("pass DescribeCdnDomainDetail %v", resp)
}

func TestModifyCdnDomain(t *testing.T) {
	client := NewTestClient()
	resp, err := client.ModifyCdnDomain(modifyDomainReq)
	if err != nil {
		t.Errorf("Failed to ModifyCdnDomain %v", err)
	}
	t.Logf("pass ModifyCdnDomain %v", resp)
}

func TestStopCdnDomain(t *testing.T) {
	client := NewTestClient()
	resp, err := client.StopCdnDomain(descDomainReq)
	if err != nil {
		t.Errorf("Failed to StopCdnDomain %v", err)
	}
	t.Logf("pass StopCdnDomain %v", resp)
}

func TestStartCdnDomain(t *testing.T) {
	client := NewTestClient()
	resp, err := client.StartCdnDomain(descDomainReq)
	if err != nil {
		t.Errorf("Failed to StartCdnDomain %v", err)
	}
	t.Logf("pass StartCdnDomain %v", resp)
}

func TestDeleteCdnDomain(t *testing.T) {
	client := NewTestClient()
	resp, err := client.DeleteCdnDomain(descDomainReq)
	if err != nil {
		t.Errorf("Failed to DeleteCdnDomain %v", err)
	}
	t.Logf("pass DeleteCdnDomain %v", resp)
}
