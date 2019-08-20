package ram

//TODO implement ram api about security
/*
	SetAccountAlias()
	GetAccountAlias()
	ClearAccountAlias()
	SetPasswordPolicy()
	GetPasswordPolicy()
*/
type AccountAliasResponse struct {
	RamCommonResponse
	AccountAlias string
}

type PasswordPolicyResponse struct {
	RamCommonResponse
	PasswordPolicy
}

type PasswordPolicyRequest struct {
	PasswordPolicy
}

type AccountAliasRequest struct {
	AccountAlias string
}

func (client *RamClient) SetAccountAlias(accountalias AccountAliasRequest) (RamCommonResponse, error) {
	var resp RamCommonResponse
	err := client.Invoke("SetAccountAlias", accountalias, &resp)
	if err != nil {
		return RamCommonResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) GetAccountAlias() (AccountAliasResponse, error) {
	var resp AccountAliasResponse
	err := client.Invoke("GetAccountAlias", struct{}{}, &resp)
	if err != nil {
		return AccountAliasResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) ClearAccountAlias() (RamCommonResponse, error) {
	var resp RamCommonResponse
	err := client.Invoke("ClearAccountAlias", struct{}{}, &resp)
	if err != nil {
		return RamCommonResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) SetPasswordPolicy(passwordPolicy PasswordPolicyRequest) (PasswordPolicyResponse, error) {
	var resp PasswordPolicyResponse
	err := client.Invoke("SetPasswordPolicy", passwordPolicy, &resp)
	if err != nil {
		return PasswordPolicyResponse{}, err
	}
	return resp, nil
}

func (client *RamClient) GetPasswordPolicy() (PasswordPolicyResponse, error) {
	var resp PasswordPolicyResponse
	err := client.Invoke("GetPasswordPolicy", struct{}{}, &resp)
	if err != nil {
		return PasswordPolicyResponse{}, err
	}
	return resp, nil
}
