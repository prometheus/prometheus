package mq

var newline string = "\n"

type SendMessage struct {
	Topic      string
	Tag        string
	ProducerId string
	Key        string
	Body       string
	Time       int64
}

type Messages struct {
	Topic      string
	Tag        string
	ConsumerId string
	Time       int64
}

type Message struct {
	Body           string `json:"body"`
	BornTime       string `json:"bornTime"`
	Key            string `json:"key"`
	MsgHandle      string `json:"msgHandle"`
	MsgId          string `json:"msgId"`
	ReconsumeTimes int    `json:"reconsumeTimes"`
	Tag            string `json:"tag"`
}

// 生产者状态码
func getStatusCodeMessage(statusCode int) string {
	switch statusCode {
	case 200:
		return ""
	case 201:
		return ""
	case 204:
		return ""
	case 400:
		return "Bad Request"
	case 403:
		return "Authentication Failed"
	case 408:
		return "Request Timeout"
	default:
		return "Unknown Error"
	}
}
