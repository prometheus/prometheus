package ram

import (
	"testing"
)

var (
	accountAliasRequest = AccountAliasRequest{AccountAlias: "hello"}
	passwordPolicy      = PasswordPolicyRequest{
		PasswordPolicy: PasswordPolicy{
			MinimumPasswordLength:      10,
			RequireLowercaseCharacters: true,
			RequireUppercaseCharacters: true,
			RequireNumbers:             true,
			RequireSymbols:             true,
		},
	}
)

func TestSetAccountAlias(t *testing.T) {
	client := NewTestClient()
	resp, err := client.SetAccountAlias(accountAliasRequest)
	if err != nil {
		t.Errorf("Failed to SetAccountAlias %v", err)
	}
	t.Logf("pass SetAccountAlias %v", resp)
}

func TestGetAccountAlias(t *testing.T) {
	client := NewTestClient()
	resp, err := client.GetAccountAlias()
	if err != nil {
		t.Errorf("Failed to GetAccountAlias %v", err)
	}
	t.Logf("pass GetAccountAlias %v", resp)
}

func TestClearAccountAlias(t *testing.T) {
	client := NewTestClient()
	resp, err := client.ClearAccountAlias()
	if err != nil {
		t.Errorf("Failed to ClearAccountAlias %v", err)
	}
	t.Logf("pass ClearAccountAlias %v", resp)
}

func TestSetPasswordPolicy(t *testing.T) {
	client := NewTestClient()
	resp, err := client.SetPasswordPolicy(passwordPolicy)
	if err != nil {
		t.Errorf("Failed to pass SetPasswordPolicy %v", err)
	}
	t.Logf("pass SetPasswordPolicy %v", resp)
}

func TestGetPasswordPolicy(t *testing.T) {
	client := NewTestClient()
	resp, err := client.GetPasswordPolicy()
	if err != nil {
		t.Errorf("Failed to pass GetPasswordPolicy %v", err)
	}
	t.Logf("pass GetPasswordPolicy %v", resp)
}
