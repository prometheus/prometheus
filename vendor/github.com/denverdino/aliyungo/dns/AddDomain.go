package dns

import (
	"log"

	"github.com/denverdino/aliyungo/common"
)

type AddDomainArgs struct {
	DomainName string

	//optional
	GroupId string
}

type AddDomainResponse struct {
	common.Response
	DomainId   string
	DomainName string
	GroupId    string
	GroupName  string
	PunyCode   string
	DnsServers struct {
		DnsServer []string
	}
}

// AddDomain
//
// You can read doc at https://help.aliyun.com/document_detail/29749.html?spm=5176.doc29805.6.592.6LMqlG
func (client *Client) AddDomain(args *AddDomainArgs) (response *AddDomainResponse, err error) {
	action := "AddDomain"
	response = &AddDomainResponse{}
	err = client.Invoke(action, args, response)
	if err == nil {
		return response, nil
	} else {
		log.Printf("%s error, %v", action, err)
		return response, err
	}
}
