package sms

import "github.com/denverdino/aliyungo/common"

//dysms是阿里云通讯的短信服务，sms是旧版本邮件推送产品短信功能，两者互不相通，必须使用对应的API。
//目前阿里已经把短信服务整合至MNS中，2017年6月22日以后开通的用户请使用MNS来发送短信。

const (
	DYSmsEndPoint   = "https://dysmsapi.aliyuncs.com/"
	SendSms         = "SendSms"
	QuerySms        = "QuerySendDetails"
	DYSmsAPIVersion = "2017-05-25"

	SmsEndPoint   = "https://sms.aliyuncs.com/"
	SingleSendSms = "SingleSendSms"
	SmsAPIVersion = "2016-09-27"
)

type Client struct {
	common.Client
}

func NewClient(accessKeyId, accessKeySecret string) *Client {
	client := new(Client)
	client.Init(SmsEndPoint, SmsAPIVersion, accessKeyId, accessKeySecret)
	return client
}

type DYSmsClient struct {
	common.Client
	Region common.Region
}

func NewDYSmsClient(accessKeyId, accessKeySecret string) *DYSmsClient {
	client := new(DYSmsClient)
	client.Init(DYSmsEndPoint, DYSmsAPIVersion, accessKeyId, accessKeySecret)
	client.Region = common.Hangzhou
	return client
}
