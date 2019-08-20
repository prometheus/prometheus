package dm

import (
	"os"
	"strconv"
	"testing"
)

func TestBatchMail(t *testing.T) {
	ID := os.Getenv("ALI_DM_ACCESS_KEY_ID")
	SECRET := os.Getenv("ALI_DM_ACCESS_KEY_SECRET")
	accountName := os.Getenv("ALI_DM_ACCOUNT_NAME")
	templateName := os.Getenv("ALI_DM_TEMPLATE_NAME")
	receiverName := os.Getenv("ALI_DM_RECEIVER_NAME")
	client := NewClient(ID, SECRET)
	err := client.SendBatchMail(&SendBatchMailArgs{SendEmailArgs: SendEmailArgs{AccountName: accountName,
		AddressType: "0"},
		TemplateName: templateName,
		ReceiverName: receiverName})
	if nil != err {
		t.Error(err.Error())
	}
}

func TestSingleMail(t *testing.T) {
	ID := os.Getenv("ALI_DM_ACCESS_KEY_ID")
	SECRET := os.Getenv("ALI_DM_ACCESS_KEY_SECRET")
	accountName := os.Getenv("ALI_DM_ACCOUNT_NAME")
	replyToAddress, _ := strconv.ParseBool(os.Getenv("ALI_DM_REPLY_TO_ADDRESS"))
	toAddress := os.Getenv("ALI_DM_TO_ADDRESS")
	client := NewClient(ID, SECRET)
	err := client.SendSingleMail(&SendSingleMailArgs{
		SendEmailArgs: SendEmailArgs{
			AccountName: accountName,
			AddressType: "0",
		},
		ReplyToAddress: replyToAddress,
		ToAddress:      toAddress})
	if nil != err {
		t.Error(err.Error())
	}
}
