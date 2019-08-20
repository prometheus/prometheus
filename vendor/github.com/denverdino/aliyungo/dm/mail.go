package dm

import (
	"net/http"

	"github.com/denverdino/aliyungo/common"
)

type SendEmailArgs struct {
	AccountName string
	AddressType string
}

type SendBatchMailArgs struct {
	SendEmailArgs
	TemplateName string
	ReceiverName string
	TagName      string
}

//remember to setup the accountName in your aliyun console
//addressType should be "1" or "0",
//0:random address, it's recommanded
//1:sender's address
//tagName is optional, you can use "" if you don't wanna use it
//please set the receiverName and template in the console of Aliyun before you call this API,if you use tagName, you should set it as well

func (this *Client) SendBatchMail(args *SendBatchMailArgs) error {
	return this.InvokeByAnyMethod(http.MethodPost, BatchSendMail, "", args, &common.Response{})
}

type SendSingleMailArgs struct {
	SendEmailArgs
	ReplyToAddress bool
	ToAddress      string
	FromAlias      string
	Subject        string
	HtmlBody       string
	TextBody       string
}

//remember to setup the accountName in your aliyun console
//addressType should be "1" or "0",
//0:random address, it's recommanded
//1:sender's address
//please set the receiverName and template in the console of Aliyun before you call this API,if you use tagName, you should set it as well

//fromAlias, subject, htmlBody, textBody are optional

func (this *Client) SendSingleMail(args *SendSingleMailArgs) error {
	return this.InvokeByAnyMethod(http.MethodPost, SingleSendMail, "", args, &common.Response{})
}
