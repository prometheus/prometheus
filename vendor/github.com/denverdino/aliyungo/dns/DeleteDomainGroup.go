package dns

import (
	"log"

	"github.com/denverdino/aliyungo/common"
)

type DeleteDomainGroupArgs struct {
	GroupId string
}

type DeleteDomainGroupResponse struct {
	common.Response
	GroupName string
}

// DeleteDomainGroup
//
// You can read doc at https://help.aliyun.com/document_detail/29764.html?spm=5176.doc29763.6.606.Vm3FyC
func (client *Client) DeleteDomainGroup(args *DeleteDomainGroupArgs) (response *DeleteDomainGroupResponse, err error) {
	action := "DeleteDomainGroup"
	response = &DeleteDomainGroupResponse{}
	err = client.Invoke(action, args, response)
	if err == nil {
		return response, nil
	} else {
		log.Printf("%s error, %v", action, err)
		return response, err
	}
}
