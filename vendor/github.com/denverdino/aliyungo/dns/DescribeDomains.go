package dns

import (
	"log"

	"github.com/denverdino/aliyungo/common"
)

type DescribeDomainsArgs struct {
	// optional
	common.Pagination
	KeyWord string
	GroupId string
}

type DescribeDomainsResponse struct {
	response common.Response
	common.PaginationResult
	Domains struct {
		Domain []DomainType
	}
}

// DescribeDomains
//
// You can read doc at https://help.aliyun.com/document_detail/29751.html?spm=5176.doc29750.6.594.dvyRJV
func (client *Client) DescribeDomains(args *DescribeDomainsArgs) (domains []DomainType, err error) {
	action := "DescribeDomains"
	response := &DescribeDomainsResponse{}
	err = client.Invoke(action, args, response)

	if err != nil {
		log.Printf("%s error, %v", action, err)
		return nil, err
	}

	return response.Domains.Domain, err
}
