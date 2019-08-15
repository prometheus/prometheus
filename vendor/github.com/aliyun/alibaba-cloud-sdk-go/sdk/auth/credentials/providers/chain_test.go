package providers

import (
	"errors"
	"testing"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
)

func TestNewChainProvider_Retrieve_UseFirstCredential(t *testing.T) {

	accessKeyCredProvider := &TestProvider{
		Credential: &credentials.AccessKeyCredential{},
		Err:        nil,
	}

	stsTokenCredProvider := &TestProvider{
		Credential: &credentials.StsTokenCredential{},
		Err:        nil,
	}

	roleCredential := &TestProvider{
		Credential: &credentials.EcsRamRoleCredential{},
		Err:        nil,
	}

	credential, err := NewChainProvider([]Provider{accessKeyCredProvider, stsTokenCredProvider, roleCredential}).Retrieve()
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := credential.(*credentials.AccessKeyCredential); !ok {
		t.Fatal("expected access key credential")
	}
}

func TestNewChainProvider_Retrieve_UseSecondCredential(t *testing.T) {

	accessKeyCredProvider := &TestProvider{
		Credential: nil,
		Err:        errors.New("I don't work"),
	}

	stsTokenCredProvider := &TestProvider{
		Credential: &credentials.StsTokenCredential{},
		Err:        nil,
	}

	roleCredential := &TestProvider{
		Credential: &credentials.EcsRamRoleCredential{},
		Err:        nil,
	}

	credential, err := NewChainProvider([]Provider{accessKeyCredProvider, stsTokenCredProvider, roleCredential}).Retrieve()
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := credential.(*credentials.StsTokenCredential); !ok {
		t.Fatal("expected sts token credential")
	}
}

func TestNewChainProvider_Retrieve_UseThirdCredential(t *testing.T) {

	accessKeyCredProvider := &TestProvider{
		Credential: nil,
		Err:        errors.New("I don't work"),
	}

	stsTokenCredProvider := &TestProvider{
		Credential: nil,
		Err:        errors.New("I don't work"),
	}

	roleCredential := &TestProvider{
		Credential: &credentials.EcsRamRoleCredential{},
		Err:        nil,
	}

	credential, err := NewChainProvider([]Provider{accessKeyCredProvider, stsTokenCredProvider, roleCredential}).Retrieve()
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := credential.(*credentials.EcsRamRoleCredential); !ok {
		t.Fatal("expected ecs ram role credential")
	}
}

func TestNewChainProvider_Retrieve_NoneWork(t *testing.T) {

	accessKeyCredProvider := &TestProvider{
		Credential: nil,
		Err:        errors.New("I don't work"),
	}

	stsTokenCredProvider := &TestProvider{
		Credential: nil,
		Err:        errors.New("I don't work"),
	}

	roleCredential := &TestProvider{
		Credential: nil,
		Err:        errors.New("I don't work"),
	}

	_, err := NewChainProvider([]Provider{accessKeyCredProvider, stsTokenCredProvider, roleCredential}).Retrieve()
	if err == nil {
		t.Fatal("expected error")
	}
}

type TestProvider struct {
	Credential auth.Credential
	Err        error
}

func (p *TestProvider) Retrieve() (auth.Credential, error) {
	return p.Credential, p.Err
}
