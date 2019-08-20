package dns

import "github.com/denverdino/aliyungo/common"

type GetMainDomainNameArgs struct {
	InputString string
}

type GetMainDomainNameResponse struct {
	common.Response
	InstanceId  string
	DomainName  string
	RR          string
	DomainLevel int32
}

// GetMainDomainName
//
// You can read doc at https://docs.aliyun.com/#/pub/dns/api-reference/domain-related&GetMainDomainName
func (client *Client) GetMainDomainName(args *GetMainDomainNameArgs) (response *GetMainDomainNameResponse, err error) {
	action := "GetMainDomainName"
	response = &GetMainDomainNameResponse{}
	err = client.Invoke(action, args, response)
	if err == nil {
		return response, nil
	} else {
		return nil, err
	}
}
