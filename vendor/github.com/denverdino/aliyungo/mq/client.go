package mq

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

type Client struct {
	AccessKey  string
	SecretKey  string
	Endpoint   string
	Topic      string
	ProducerId string
	ConsumerId string
	Key        string
	Tag        string
}

type MessageResponse struct {
	Body     string `json:"body"`
	BornTime int64  `json:"bornTime"` // UTC time in Unix
}

func NewClient(ak string, sk string, endpoint string, topic string,
	producerId string, consumerId string, key string, tag string) (client *Client) {
	client = &Client{
		AccessKey:  ak,
		SecretKey:  sk,
		Endpoint:   endpoint,
		Topic:      topic,
		ProducerId: producerId,
		ConsumerId: consumerId,
		Key:        key,
		Tag:        tag,
	}
	return client
}

func getSendUrl(endpoint string, topic string, time int64, tag string, key string) string {
	return endpoint + "/message/?topic=" + topic + "&time=" +
		strconv.FormatInt(time, 10) + "&tag=" + tag + "&key=" + key
}

func getReceiveUrl(endpoint, topic string, time int64, tag string, num int) string {
	return endpoint + "/message/?topic=" + topic + "&time=" +
		strconv.FormatInt(time, 10) + "&tag=" + tag + "&num=" + strconv.Itoa(num)
}

func getSendSign(topic string, producerId string, messageBody []byte, time int64, sk string) (sign string) {
	signStr := topic + newline + producerId + newline + Md5(messageBody) + newline + strconv.FormatInt(time, 10)
	sign = HamSha1(signStr, []byte(sk))
	return sign
}

func getReceiveSign(topic string, consumerId string, time int64, sk string) string {
	// [topic+”\n”+ cid+”\n”+time]
	signStr := topic + newline + consumerId + newline + strconv.FormatInt(time, 10)
	return HamSha1(signStr, []byte(sk))
}

func getReceiveHeader(ak, sign, consumerId string) (map[string]string, error) {
	if consumerId == "" {
		return nil, fmt.Errorf("consumer id is not provided")
	}
	header := make(map[string]string)
	header["AccessKey"] = ak
	header["Signature"] = sign
	header["ConsumerId"] = consumerId
	return header, nil
}

func getSendHeader(ak string, sign string, producerId string) (header map[string]string, err error) {
	if producerId == "" {
		return nil, fmt.Errorf("producer id is not provided")
	}
	header = make(map[string]string, 0)
	header["AccessKey"] = ak
	header["Signature"] = sign
	header["ProducerId"] = producerId
	return header, nil
}

func (client *Client) Send(time int64, message []byte) (msgId string, err error) {
	url := getSendUrl(client.Endpoint, client.Topic, time, client.Tag, client.Key)
	sign := getSendSign(client.Topic, client.ProducerId, message, time, client.SecretKey)
	header, err := getSendHeader(client.AccessKey, sign, client.ProducerId)
	if err != nil {
		return "", err
	}
	response, status, err := httpPost(url, header, message)
	if err != nil {
		return "", err
	}

	fmt.Printf("receive message: %s %d", response, status)
	statusMessage := getStatusCodeMessage(status)
	if statusMessage != "" {
		return "", errors.New(statusMessage)
	}

	var rs interface{}
	err = json.Unmarshal(response, &rs)
	if err != nil {
		return "", err
	}

	result := rs.(map[string]interface{})

	sendStatus := result["sendStatus"].(string)
	if sendStatus != "SEND_OK" {
		return "", errors.New(sendStatus)
	}

	return result["msgId"].(string), nil
}

func (client *Client) ReceiveMessage(messageChan chan string, errChan chan error) {
	// only listen for the latest message
	time := GetCurrentUnixMicro()
	url := getReceiveUrl(client.Endpoint, client.Topic, time, client.Tag, 1)
	sign := getReceiveSign(client.Topic, client.ConsumerId, time, client.SecretKey)
	header, err := getReceiveHeader(client.AccessKey, sign, client.ConsumerId)
	if err != nil {
		errChan <- err
		return
	}
	response, status, err := HttpGet(url, header)
	if err != nil {
		errChan <- err
		return
	}

	fmt.Printf("receive message: %s %d", response, status)
	statusMessage := getStatusCodeMessage(status)
	if statusMessage != "" {
		errChan <- errors.New(statusMessage)
		return
	}

	messages := make([]MessageResponse, 0)
	json.Unmarshal(response, &messages)

	if len(messages) > 0 {
		fmt.Printf("size of messages is %d", len(messages))
		message := messages[0]
		messageChan <- message.Body
	} else {
		errChan <- fmt.Errorf("no message available")
		return
	}
}
