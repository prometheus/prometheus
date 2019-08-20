package dns

import (
	"log"

	"github.com/denverdino/aliyungo/common"
)

type DomainType struct {
	DomainId    string
	DomainName  string
	AliDomain   bool
	GroupId     string
	GroupName   string
	InstanceId  string
	VersionCode string
	PunyCode    string
	DnsServers  struct {
		DnsServer []string
	}
}

type DescribeDomainInfoArgs struct {
	DomainName string
}

type DescribeDomainInfoResponse struct {
	response common.Response
	DomainType
}

// DescribeDomainInfo
//
// You can read doc at https://help.aliyun.com/document_detail/29752.html?spm=5176.doc29751.6.595.VJM3Gy
func (client *Client) DescribeDomainInfo(args *DescribeDomainInfoArgs) (domain DomainType, err error) {
	action := "DescribeDomainInfo"
	response := &DescribeDomainInfoResponse{}
	err = client.Invoke(action, args, response)

	if err != nil {
		log.Printf("%s error, %v", action, err)
		return DomainType{}, err
	}

	return response.DomainType, nil
}
