package mns

type Queue struct {
	*Client
	QueueName string
	Base64    bool
}

type Message struct {
	MessageBody string `xml:"MessageBody"`
}

type MsgSend struct {
	MessageId      string `xml:"MessageId"`
	MessageBodyMD5 string `xml:"MessageBodyMD5"`
}

type MsgReceive struct {
	MessageId       string `xml:"MessageId"`
	MessageBodyMD5  string `xml:"MessageBodyMD5"`
	MessageBody     string `xml:"MessageBody"`
	ReceiptHandle   string `xml:"ReceiptHandle"`
	EnqueueTime     int64  `xml:"EnqueueTime"`
	NextVisibleTime int64  `xml:"NextVisibleTime"`
	DequeueCount    int    `xml:"DequeueCount"`
	Priority        int    `xml:"Priority"`
}
