package kms

import "testing"

func TestClient_Encrypt(t *testing.T) {
	args := &EncryptAgrs{
		KeyId:             TestKeyId,
		Plaintext:         "abc",
		EncryptionContext: encryptionContext,
	}

	response, err := debugClient.Encrypt(args)
	if err != nil {
		t.Fatalf("Error %++v", err)
	} else {
		t.Logf("Result = %++v", response)
	}
}
