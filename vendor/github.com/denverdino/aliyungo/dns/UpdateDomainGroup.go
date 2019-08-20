package dns

import (
	"log"

	"github.com/denverdino/aliyungo/common"
)

type UpdateDomainGroupArgs struct {
	GroupId   string
	GroupName string
}

type UpdateDomainGroupResponse struct {
	common.Response
	GroupId   string
	GroupName string
}

// UpdateDomainGroup
//
// You can read doc at https://help.aliyun.com/document_detail/29763.html?spm=5176.doc29762.6.605.iFRKjn
func (client *Client) UpdateDomainGroup(args *UpdateDomainGroupArgs) (response *UpdateDomainGroupResponse, err error) {
	action := "UpdateDomainGroup"
	response = &UpdateDomainGroupResponse{}
	err = client.Invoke(action, args, response)
	if err == nil {
		return response, nil
	} else {
		log.Printf("%s error, %v", action, err)
		return response, err
	}
}
