package mns

import (
	"encoding/xml"
	"os"
	"testing"

	"github.com/denverdino/aliyungo/common"
)

var (
	TestAccessKeyId     = os.Getenv("AccessKeyId")
	TestAccessKeySecret = os.Getenv("AccessKeySecret")
	TestSecurityToken   = os.Getenv("SecurityToken")
	TestEndpoint        = os.Getenv("Endpoint")
	TestQueueName       = os.Getenv("QueueName")
	TestRegionID        = common.Region(os.Getenv("RegionId"))
)

type MessageData struct {
	MessageBody string
}

func TestNewClient(t *testing.T) {
	client := NewClient(TestAccessKeyId, TestAccessKeySecret, TestEndpoint)
	queue := Queue{
		Client:    client,
		QueueName: TestQueueName,
		Base64:    false,
	}

	msg := Message{MessageBody: "MessageBody"}
	data, err := xml.Marshal(msg)
	if err != nil {
		t.Error(err)
	}

	msgId, err := queue.Send(GetCurrentUnixMicro(), data)
	if err != nil {
		t.Error(err)
	} else {
		t.Logf("send message %v", msgId)
	}
}

func TestNewClientForAssumeRole(t *testing.T) {
	client := NewClientForAssumeRole(TestAccessKeyId, TestAccessKeySecret, TestSecurityToken, TestEndpoint)
	queue := Queue{
		Client:    client,
		QueueName: TestQueueName,
		Base64:    false,
	}

	msg := Message{MessageBody: "MessageBody"}
	data, err := xml.Marshal(msg)
	if err != nil {
		t.Error(err)
	}

	msgId, err := queue.Send(GetCurrentUnixMicro(), data)
	if err != nil {
		t.Error(err)
	} else {
		t.Logf("send message %v", msgId)
	}
}

func TestReceiveClient(t *testing.T) {

	// time.Sleep(100 * time.Second)
	client := NewClient(TestAccessKeyId, TestAccessKeySecret, TestEndpoint)
	queue := Queue{
		Client:    client,
		QueueName: TestQueueName,
		Base64:    false,
	}
	respChan := make(chan MsgReceive)
	errChan := make(chan error)
	end := make(chan int)
	receiptHandle := ""
	go func() {
		select {
		case resp := <-respChan:
			{
				t.Logf("receive message: %v", resp)
				receiptHandle = resp.ReceiptHandle
				end <- 1
			}
		case err := <-errChan:
			{
				t.Error(err)
				end <- 0
			}
		}
	}()

	queue.Receive(respChan, errChan)
	received := <-end

	if received == 1 {
		msgDelete(receiptHandle, t)
	}
}

func msgDelete(receiptHandle string, t *testing.T) {
	client := NewClient(TestAccessKeyId, TestAccessKeySecret, TestEndpoint)
	queue := Queue{
		Client:    client,
		QueueName: TestQueueName,
		Base64:    false,
	}
	errChan := make(chan error)
	end := make(chan int)
	go func() {
		select {
		case err := <-errChan:
			{
				if err != nil {
					t.Error(err)
					end <- 0
				} else {
					t.Log("deletesuccess" + receiptHandle)
					end <- 1
				}
			}
		}
	}()

	queue.Delete(receiptHandle, errChan)
	<-end
}
