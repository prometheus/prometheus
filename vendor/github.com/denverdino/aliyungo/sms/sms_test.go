package sms

import (
	"os"
	"testing"
)

func TestSms(t *testing.T) {
	ID := os.Getenv("ALI_DM_ACCESS_KEY_ID")
	SECRET := os.Getenv("ALI_DM_ACCESS_KEY_SECRET")
	SIGNAME := os.Getenv("ALI_DM_SMS_SIGN_NAME")
	TEMPCODE := os.Getenv("ALI_DM_SMS_TEMPLATE_CODE")
	NUM := os.Getenv("ALI_DM_SMS_TEST_PHONE")
	client := NewDYSmsClient(ID, SECRET)
	client.SendSms(&SendSmsArgs{
		SignName:      SIGNAME,
		TemplateCode:  TEMPCODE,
		PhoneNumbers:  NUM,
		TemplateParam: `{"number": "123"}`,
	})
}
