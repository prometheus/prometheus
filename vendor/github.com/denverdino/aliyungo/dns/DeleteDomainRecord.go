package dns

import "github.com/denverdino/aliyungo/common"

type DeleteDomainRecordArgs struct {
	RecordId string
}

type DeleteDomainRecordResponse struct {
	common.Response
	InstanceId string
	RecordId   string
}

// DeleteDomainRecord
//
// You can read doc at https://docs.aliyun.com/#/pub/dns/api-reference/record-related&DeleteDomainRecord
func (client *Client) DeleteDomainRecord(args *DeleteDomainRecordArgs) (response *DeleteDomainRecordResponse, err error) {
	action := "DeleteDomainRecord"
	response = &DeleteDomainRecordResponse{}
	err = client.Invoke(action, args, response)
	if err == nil {
		return response, nil
	} else {
		return nil, err
	}
}
