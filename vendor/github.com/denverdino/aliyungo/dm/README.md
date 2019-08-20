# aliyun direct mail

这个服务支持通过阿里云发送短信以及邮件。
使用接口前需要在控制台设置相关的模版和签名，具体参考DM服务的说明文档:

[Direct Mail](https://help.aliyun.com/document_detail/29414.html)

调用方法如测试文件中的一样

```go
func TestSms(t *testing.T) {
	ID := os.Getenv("ALI_DM_ACCESS_KEY_ID")
	SECRET := os.Getenv("ALI_DM_ACCESS_KEY_SECRET")
	SIGNAME := os.Getenv("ALI_DM_SMS_SIGN_NAME")
	TEMPCODE := os.Getenv("ALI_DM_SMS_TEMPLATE_CODE")
	NUM := os.Getenv("ALI_DM_SMS_TEST_PHONE")
	client := NewClient(ID, SECRET)
	client.SendSms(SIGNAME, TEMPCODE, NUM, map[string]string{
		"number": "123456",
	})
}
```

测试这个包，需要设置下面用到的环境变量
```go
	ID := os.Getenv("ALI_DM_ACCESS_KEY_ID")
	SECRET := os.Getenv("ALI_DM_ACCESS_KEY_SECRET")
	SIGNAME := os.Getenv("ALI_DM_SMS_SIGN_NAME")
	TEMPCODE := os.Getenv("ALI_DM_SMS_TEMPLATE_CODE")
	NUM := os.Getenv("ALI_DM_SMS_TEST_PHONE")
	accountName := os.Getenv("ALI_DM_ACCOUNT_NAME")
	templateName := os.Getenv("ALI_DM_TEMPLATE_NAME")
	receiverName := os.Getenv("ALI_DM_RECEIVER_NAME")
	accountName := os.Getenv("ALI_DM_ACCOUNT_NAME")
	replyToAddress := os.Getenv("ALI_DM_REPLY_TO_ADDRESS")
	toAddress := os.Getenv("ALI_DM_TO_ADDRESS")
```

maintainer: johnzeng

#2016.12.20
将短信API从邮件中分离了出来，并且让DM服务使用Common Client，采用struct形式的参数，与其他服务统一。具体使用方法参见mail_test。
yarous224