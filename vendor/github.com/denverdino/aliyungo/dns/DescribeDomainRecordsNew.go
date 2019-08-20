package dns

import "github.com/denverdino/aliyungo/common"

type DescribeDomainRecordsNewArgs struct {
	DomainName string

	//optional
	common.Pagination
	RRKeyWord    string
	TypeKeyWord  string
	ValueKeyWord string
}

type DescribeDomainRecordsNewResponse struct {
	common.Response
	common.PaginationResult
	InstanceId    string
	DomainRecords struct {
		Record []RecordTypeNew
	}
}

// DescribeDomainRecordsNew
//
// You can read doc at https://docs.aliyun.com/#/pub/dns/api-reference/record-related&DescribeDomainRecords
func (client *Client) DescribeDomainRecordsNew(args *DescribeDomainRecordsNewArgs) (response *DescribeDomainRecordsNewResponse, err error) {
	action := "DescribeDomainRecords"
	response = &DescribeDomainRecordsNewResponse{}
	err = client.Invoke(action, args, response)
	if err == nil {
		return response, nil
	} else {
		return nil, err
	}
}
