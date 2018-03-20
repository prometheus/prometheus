package api

import (
	"fmt"
	"github.com/sacloud/libsacloud/sacloud"
)

// SSHKeyAPI 公開鍵API
type SSHKeyAPI struct {
	*baseAPI
}

// NewSSHKeyAPI 公開鍵API作成
func NewSSHKeyAPI(client *Client) *SSHKeyAPI {
	return &SSHKeyAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "sshkey"
			},
		},
	}
}

// Generate 公開鍵の作成
func (api *SSHKeyAPI) Generate(name string, passPhrase string, desc string) (*sacloud.SSHKeyGenerated, error) {

	var (
		method = "POST"
		uri    = fmt.Sprintf("%s/generate", api.getResourceURL())
	)

	type genRequest struct {
		Name           string
		GenerateFormat string
		Description    string
		PassPhrase     string
	}

	type request struct {
		SSHKey genRequest
	}
	type response struct {
		*sacloud.ResultFlagValue
		SSHKey *sacloud.SSHKeyGenerated
	}

	body := &request{
		SSHKey: genRequest{
			Name:           name,
			GenerateFormat: "openssh",
			PassPhrase:     passPhrase,
			Description:    desc,
		},
	}

	res := &response{}

	_, err := api.action(method, uri, body, res)
	if err != nil {
		return nil, fmt.Errorf("SSHKeyAPI: generate SSHKey is failed: %s", err)
	}
	return res.SSHKey, nil
}
