package dns

import "github.com/denverdino/aliyungo/common"

type DeleteSubDomainRecordsArgs struct {
	DomainName string
	RR         string

	//optional
	Type string
}

type DeleteSubDomainRecordsResponse struct {
	common.Response
	InstanceId string
	RR         string
	//	TotalCount int32
}

// DeleteSubDomainRecords
//
// You can read doc at https://docs.aliyun.com/#/pub/dns/api-reference/record-related&DeleteSubDomainRecords
func (client *Client) DeleteSubDomainRecords(args *DeleteSubDomainRecordsArgs) (response *DeleteSubDomainRecordsResponse, err error) {
	action := "DeleteSubDomainRecords"
	response = &DeleteSubDomainRecordsResponse{}
	err = client.Invoke(action, args, response)
	if err == nil {
		return response, nil
	} else {
		return nil, err
	}
}
