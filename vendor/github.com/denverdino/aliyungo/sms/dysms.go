package sms

import (
	"net/http"

	"github.com/denverdino/aliyungo/common"
)

//阿里云通信
type SendSmsArgs struct {
	PhoneNumbers    string
	SignName        string
	TemplateCode    string
	TemplateParam   string
	SmsUpExtendCode string `ArgName:"smsUpExtendCode"`
	OutId           string
}

type QuerySmsArgs struct {
	PhoneNumber string
	BizId       string
	SendDate    string
	PageSize    string
	CurrentPage string
}

type SendSmsResponse struct {
	common.Response
	Code    string
	Message string
	BizId   string
}

type QuerySmsResponse struct {
	common.Response
	Code              string
	Message           string
	TotalCount        int
	TotalPage         string
	SmsSendDetailDTOs struct {
		SmsSendDetailDTO []SmsSendDetailDTOsItem
	}
}

type SmsSendDetailDTOsItem struct {
	PhoneNum     string
	SendStatus   int
	ErrCode      string
	TemplateCode string
	Content      string
	SendDate     string
	ReceiveDate  string
	OutId        string
}

func (this *DYSmsClient) SendSms(args *SendSmsArgs) (*SendSmsResponse, error) {
	resp := SendSmsResponse{}
	return &resp, this.InvokeByAnyMethod(http.MethodGet, SendSms, "", args, &resp)
}

func (this *DYSmsClient) QuerySms(args *QuerySmsArgs) (*QuerySmsResponse, error) {
	resp := QuerySmsResponse{}
	return &resp, this.InvokeByAnyMethod(http.MethodGet, QuerySms, "", args, &resp)
}
