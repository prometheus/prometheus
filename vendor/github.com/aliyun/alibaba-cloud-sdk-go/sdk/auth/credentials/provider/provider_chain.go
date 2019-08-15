package provider

import (
	"errors"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth"
)

type ProviderChain struct {
	Providers []Provider
}

var defaultproviders = []Provider{ProviderEnv, ProviderProfile, ProviderInstance}
var DefaultChain = NewProviderChain(defaultproviders)

func NewProviderChain(providers []Provider) Provider {
	return &ProviderChain{
		Providers: providers,
	}
}

func (p *ProviderChain) Resolve() (auth.Credential, error) {
	for _, provider := range p.Providers {
		creds, err := provider.Resolve()
		if err != nil {
			return nil, err
		} else if err == nil && creds == nil {
			continue
		}
		return creds, err
	}
	return nil, errors.New("No credential found")

}
