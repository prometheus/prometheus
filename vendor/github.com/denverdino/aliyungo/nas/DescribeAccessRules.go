package nas

type DescribeAccessRulesRequest struct {
	AccessGroupName string
	Version         string
	RegionId        string
}

type DescribeAccessRulesResponse struct {
	Rules []Rule
	Code  string
}

type Rule struct {
	Priority     string
	SourceCidrIp string
	SquashType   string
	RuleId       string
	Policy       string
}

func (client *Client) DescribeAccessRules(args *DescribeAccessRulesRequest) (resp DescribeAccessRulesResponse, err error) {
	response := DescribeAccessRulesResponse{}
	args.Version = VERSION
	err = client.Invoke("DescribeAccessRules", args, &response)
	return response, err
}
