package nas

type CreateAccessRuleRequest struct {
	AccessGroupName string
	SourceCidrIp    string
	Policy          string
	SquashType      string
	Priority        string
	Version         string
	RegionId        string
}

type CreateAccessRuleResponse struct {
	Code string
}

func (client *Client) CreateAccessRule(args *CreateAccessRuleRequest) (resp CreateAccessRuleResponse, err error) {
	response := CreateAccessRuleResponse{}
	args.Version = VERSION
	args.Policy = DEFAULT_POLICY
	args.SquashType = DEFAULT_SQUASHTYPE
	args.Priority = DEFAULT_PRIORITY

	err = client.Invoke("CreateAccessRule", args, &response)
	return response, err
}
