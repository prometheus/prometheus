package providers

import (
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth"
)

type Provider interface {
	Retrieve() (auth.Credential, error)
}

// NewChainProvider will attempt to use its given providers in the order
// in which they're provided. It will return credentials for the first
// provider that doesn't return an error.
func NewChainProvider(providers []Provider) Provider {
	return &ChainProvider{
		Providers: providers,
	}
}

type ChainProvider struct {
	Providers []Provider
}

func (p *ChainProvider) Retrieve() (auth.Credential, error) {
	var lastErr error
	for _, provider := range p.Providers {
		creds, err := provider.Retrieve()
		if err == nil {
			return creds, nil
		}
		lastErr = err
	}
	return nil, lastErr
}
