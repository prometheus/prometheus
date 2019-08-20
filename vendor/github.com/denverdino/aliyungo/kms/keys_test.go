package kms

import (
	"fmt"
	"testing"
	"time"

	"os"

	"github.com/denverdino/aliyungo/common"
)

func TestClient_CreateKey(t *testing.T) {
	args := &CreateKeyArgs{
		KeyUsage:    KEY_USAGE_ENCRYPT_DECRYPT,
		Description: fmt.Sprintf("my-test-key-%d", time.Now().Unix()),
	}

	response, err := debugClient.CreateKey(args)
	if err != nil {
		t.Fatalf("Error %++v", err)
	} else {
		t.Logf("Result = %++v", response)
	}
}

func TestClient_DescribeKey(t *testing.T) {
	response, err := debugClient.DescribeKey(TestKeyId)
	if err != nil {
		t.Fatalf("Error %++v", err)
	} else {
		t.Logf("Result = %++v", response)
	}
}

func TestClient_DisableKey(t *testing.T) {
	response, err := debugClient.DisableKey(TestKeyId)
	if err != nil {
		t.Fatalf("Error %++v", err)
	} else {
		t.Logf("Result = %++v", response)
	}
}

func TestClient_EnableKey(t *testing.T) {
	response, err := debugClient.EnableKey(TestKeyId)
	if err != nil {
		t.Fatalf("Error %++v", err)
	} else {
		t.Logf("Result = %++v", response)
	}
}

func TestClient_GenerateDataKey(t *testing.T) {
	args := &GenerateDataKeyArgs{
		KeyId:             TestKeyId,
		KeySpec:           KeySpec_AES_256,
		NumberOfBytes:     512,
		EncryptionContext: encryptionContext,
	}

	response, err := debugClient.GenerateDataKey(args)
	if err != nil {
		t.Fatalf("Error %++v", err)
	} else {
		t.Logf("Result = %++v", response)
	}
}

func TestClient_ListKeys(t *testing.T) {
	args := &ListKeysArgs{
		Pagination: common.Pagination{
			PageNumber: 1,
			PageSize:   100,
		},
	}
	response, err := debugClient.ListKeys(args)
	if err != nil {
		t.Fatalf("Error %++v", err)
	} else {
		t.Logf("Result = %++v", response)
	}
}

func TestClient_ScheduleKeyDeletion(t *testing.T) {
	args := &ScheduleKeyDeletionArgs{
		KeyId:               os.Getenv("ScheduleKeyDeletionKeyId"),
		PendingWindowInDays: 7,
	}
	response, err := debugClient.ScheduleKeyDeletion(args)
	if err != nil {
		t.Fatalf("Error %++v", err)
	} else {
		t.Logf("Result = %++v", response)
	}
}

func TestClient_CancelKeyDeletion(t *testing.T) {

	response, err := debugClient.CancelKeyDeletion(os.Getenv("ScheduleKeyDeletionKeyId"))
	if err != nil {
		t.Fatalf("Error %++v", err)
	} else {
		t.Logf("Result = %++v", response)
	}
}
