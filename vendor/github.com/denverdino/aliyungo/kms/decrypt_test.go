package kms

import "testing"

func TestClient_Decrypt(t *testing.T) {
	encryptArgs := &EncryptAgrs{
		KeyId:             TestKeyId,
		EncryptionContext: encryptionContext,
		Plaintext:         "abc",
	}

	encryptResponse, err := debugClient.Encrypt(encryptArgs)
	if err != nil {
		t.Fatalf("Error %++v", err)
	} else {
		t.Logf("Result = %++v", encryptResponse)
	}

	args := &DecryptArgs{
		CiphertextBlob:    encryptResponse.CiphertextBlob,
		EncryptionContext: encryptionContext,
	}

	response, err := debugClient.Decrypt(args)
	if err != nil {
		t.Fatalf("Error %++v", err)
	} else {
		t.Logf("Result = %++v", response)
	}

}
