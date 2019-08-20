package dns

import (
	"log"

	"github.com/denverdino/aliyungo/common"
)

type DomainGroupType struct {
	GroupId   string
	GroupName string
}

type DescribeDomainGroupsArgs struct {
	//optional
	common.Pagination
	KeyWord string
}

type DescribeDomainGroupsResponse struct {
	response common.Response
	common.PaginationResult
	DomainGroups struct {
		DomainGroup []DomainGroupType
	}
}

// DescribeDomainGroups
//
// You can read doc at https://help.aliyun.com/document_detail/29766.html?spm=5176.doc29765.6.608.qcQr2R
func (client *Client) DescribeDomainGroups(args *DescribeDomainGroupsArgs) (groups []DomainGroupType, err error) {
	action := "DescribeDomainGroups"
	response := &DescribeDomainGroupsResponse{}
	err = client.Invoke(action, args, response)

	if err != nil {
		log.Printf("%s error, %v", action, err)
		return nil, err
	}

	return response.DomainGroups.DomainGroup, nil
}
