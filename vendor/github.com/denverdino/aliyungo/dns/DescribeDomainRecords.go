package dns

import "github.com/denverdino/aliyungo/common"

type DescribeDomainRecordsArgs struct {
	DomainName string

	//optional
	common.Pagination
	RRKeyWord    string
	TypeKeyWord  string
	ValueKeyWord string
}

type DescribeDomainRecordsResponse struct {
	common.Response
	common.PaginationResult
	InstanceId    string
	DomainRecords struct {
		Record []RecordType
	}
}

// DescribeDomainRecords
//
// You can read doc at https://docs.aliyun.com/#/pub/dns/api-reference/record-related&DescribeDomainRecords
func (client *Client) DescribeDomainRecords(args *DescribeDomainRecordsArgs) (response *DescribeDomainRecordsResponse, err error) {
	action := "DescribeDomainRecords"
	response = &DescribeDomainRecordsResponse{}
	err = client.Invoke(action, args, response)
	if err == nil {
		return response, nil
	} else {
		return nil, err
	}
}
