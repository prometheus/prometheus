package mns

import (
	"encoding/xml"
	"io/ioutil"
	"net/http"
	"strconv"
)

//队列接口PATH
//POST /queues/$queueName/messages HTTP/1.1
//GET /queues/$queueName/messages?waitseconds=10 HTTP/1.1
//DELETE /queues/$queueName/messages?ReceiptHandle=<receiptHandle> HTTP/1.1
func getPath(queue string) string {
	return "/queues/" + queue + "/messages"
}

//发送队列消息
func (queue *Queue) Send(time int64, message []byte) (msg MsgSend, err error) {
	req := &request{
		endpoint:    queue.Endpoint,
		method:      http.MethodPost,
		path:        getPath(queue.QueueName),
		payload:     message,
		contentType: "text/xml",
		headers:     map[string]string{},
	}

	response, err := queue.doRequest(req)
	if err != nil {
		return
	}

	defer response.Body.Close()
	//err = xml.NewDecoder(response.Body).Decode(msg)

	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return
	}
	//fmt.Printf("receive message: %s \n", data)
	err = xml.Unmarshal(data, &msg)

	if err != nil {
		return
	}

	return

}

//消费队列消息
func (queue *Queue) Receive(messageChan chan MsgReceive, errChan chan error) {
	req := &request{
		endpoint: queue.Endpoint,
		method:   http.MethodGet,
		path:     getPath(queue.QueueName),
		params: map[string]string{
			"waitseconds": strconv.Itoa(5),
		},
		payload:     nil,
		contentType: "text/xml",
		headers:     map[string]string{},
	}

	response, err := queue.doRequest(req)
	if err != nil {
		errChan <- err
		return
	}

	defer response.Body.Close()
	rs := MsgReceive{}
	//err = xml.NewDecoder(response.Body).Decode(rs)

	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		errChan <- err
		return
	}
	//fmt.Printf("receive message: %s \n", data)
	err = xml.Unmarshal(data, &rs)

	if err != nil {
		errChan <- err
		return
	}

	messageChan <- rs
	return
}

//删除队列消息
func (queue *Queue) Delete(receiptHandle string, errChan chan error) {
	req := &request{
		endpoint: queue.Endpoint,
		method:   http.MethodDelete,
		path:     getPath(queue.QueueName),
		params: map[string]string{
			"ReceiptHandle": receiptHandle,
		},
		payload:     nil,
		contentType: "text/xml",
		headers:     map[string]string{},
	}

	_, err := queue.doRequest(req)
	if err != nil {
		errChan <- err
		return
	}

	errChan <- nil
	return
}
