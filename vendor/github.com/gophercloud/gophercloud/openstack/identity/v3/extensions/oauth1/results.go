package oauth1

import (
	"encoding/json"
	"net/url"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/pagination"
)

// Consumer represents a delegated authorization request between two
// identities.
type Consumer struct {
	ID          string `json:"id"`
	Secret      string `json:"secret"`
	Description string `json:"description"`
}

type consumerResult struct {
	gophercloud.Result
}

// CreateConsumerResult is the response from a Create operation. Call its
// Extract method to interpret it as a Consumer.
type CreateConsumerResult struct {
	consumerResult
}

// UpdateConsumerResult is the response from a Create operation. Call its
// Extract method to interpret it as a Consumer.
type UpdateConsumerResult struct {
	consumerResult
}

// DeleteConsumerResult is the response from a Delete operation. Call its
// ExtractErr to determine if the request succeeded or failed.
type DeleteConsumerResult struct {
	gophercloud.ErrResult
}

// ConsumersPage is a single page of Region results.
type ConsumersPage struct {
	pagination.LinkedPageBase
}

// GetConsumerResult is the response from a Get operation. Call its Extract
// method to interpret it as a Consumer.
type GetConsumerResult struct {
	consumerResult
}

// IsEmpty determines whether or not a page of Consumers contains any results.
func (c ConsumersPage) IsEmpty() (bool, error) {
	consumers, err := ExtractConsumers(c)
	return len(consumers) == 0, err
}

// NextPageURL extracts the "next" link from the links section of the result.
func (c ConsumersPage) NextPageURL() (string, error) {
	var s struct {
		Links struct {
			Next     string `json:"next"`
			Previous string `json:"previous"`
		} `json:"links"`
	}
	err := c.ExtractInto(&s)
	if err != nil {
		return "", err
	}
	return s.Links.Next, err
}

// ExtractConsumers returns a slice of Consumers contained in a single page of
// results.
func ExtractConsumers(r pagination.Page) ([]Consumer, error) {
	var s struct {
		Consumers []Consumer `json:"consumers"`
	}
	err := (r.(ConsumersPage)).ExtractInto(&s)
	return s.Consumers, err
}

// Extract interprets any consumer result as a Consumer.
func (c consumerResult) Extract() (*Consumer, error) {
	var s struct {
		Consumer *Consumer `json:"consumer"`
	}
	err := c.ExtractInto(&s)
	return s.Consumer, err
}

// Token contains an OAuth1 token.
type Token struct {
	// OAuthToken is the key value for the oauth token that the Identity API returns.
	OAuthToken string `q:"oauth_token"`
	// OAuthTokenSecret is the secret value associated with the OAuth Token.
	OAuthTokenSecret string `q:"oauth_token_secret"`
	// OAuthExpiresAt is the date and time when an OAuth token expires.
	OAuthExpiresAt *time.Time `q:"-"`
}

// TokenResult is a struct to handle
// "Content-Type: application/x-www-form-urlencoded" response.
type TokenResult struct {
	gophercloud.Result
	Body []byte
}

// Extract interprets any OAuth1 token result as a Token.
func (r TokenResult) Extract() (*Token, error) {
	if r.Err != nil {
		return nil, r.Err
	}

	values, err := url.ParseQuery(string(r.Body))
	if err != nil {
		return nil, err
	}

	token := &Token{
		OAuthToken:       values.Get("oauth_token"),
		OAuthTokenSecret: values.Get("oauth_token_secret"),
	}

	if v := values.Get("oauth_expires_at"); v != "" {
		if t, err := time.Parse(gophercloud.RFC3339Milli, v); err != nil {
			return nil, err
		} else {
			token.OAuthExpiresAt = &t
		}
	}

	return token, nil
}

// AuthorizedToken contains an OAuth1 authorized token info.
type AuthorizedToken struct {
	// OAuthVerifier is the ID of the token verifier.
	OAuthVerifier string `json:"oauth_verifier"`
}

type AuthorizeTokenResult struct {
	gophercloud.Result
}

// Extract interprets AuthorizeTokenResult result as a AuthorizedToken.
func (r AuthorizeTokenResult) Extract() (*AuthorizedToken, error) {
	var s struct {
		AuthorizedToken *AuthorizedToken `json:"token"`
	}
	err := r.ExtractInto(&s)
	return s.AuthorizedToken, err
}

// AccessToken represents an AccessToken response as a struct.
type AccessToken struct {
	ID                string     `json:"id"`
	ConsumerID        string     `json:"consumer_id"`
	ProjectID         string     `json:"project_id"`
	AuthorizingUserID string     `json:"authorizing_user_id"`
	ExpiresAt         *time.Time `json:"-"`
}

func (r *AccessToken) UnmarshalJSON(b []byte) error {
	type tmp AccessToken
	var s struct {
		tmp
		ExpiresAt *gophercloud.JSONRFC3339Milli `json:"expires_at"`
	}
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}
	*r = AccessToken(s.tmp)

	if s.ExpiresAt != nil {
		t := time.Time(*s.ExpiresAt)
		r.ExpiresAt = &t
	}

	return nil
}

type GetAccessTokenResult struct {
	gophercloud.Result
}

// Extract interprets any GetAccessTokenResult result as an AccessToken.
func (r GetAccessTokenResult) Extract() (*AccessToken, error) {
	var s struct {
		AccessToken *AccessToken `json:"access_token"`
	}
	err := r.ExtractInto(&s)
	return s.AccessToken, err
}

// RevokeAccessTokenResult is the response from a Delete operation. Call its
// ExtractErr to determine if the request succeeded or failed.
type RevokeAccessTokenResult struct {
	gophercloud.ErrResult
}

// AccessTokensPage is a single page of Access Tokens results.
type AccessTokensPage struct {
	pagination.LinkedPageBase
}

// IsEmpty determines whether or not a an AccessTokensPage contains any results.
func (r AccessTokensPage) IsEmpty() (bool, error) {
	accessTokens, err := ExtractAccessTokens(r)
	return len(accessTokens) == 0, err
}

// NextPageURL extracts the "next" link from the links section of the result.
func (r AccessTokensPage) NextPageURL() (string, error) {
	var s struct {
		Links struct {
			Next     string `json:"next"`
			Previous string `json:"previous"`
		} `json:"links"`
	}
	err := r.ExtractInto(&s)
	if err != nil {
		return "", err
	}
	return s.Links.Next, err
}

// ExtractAccessTokens returns a slice of AccessTokens contained in a single
// page of results.
func ExtractAccessTokens(r pagination.Page) ([]AccessToken, error) {
	var s struct {
		AccessTokens []AccessToken `json:"access_tokens"`
	}
	err := (r.(AccessTokensPage)).ExtractInto(&s)
	return s.AccessTokens, err
}

// AccessTokenRole represents an Access Token Role struct.
type AccessTokenRole struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	DomainID string `json:"domain_id"`
}

// AccessTokenRolesPage is a single page of Access Token roles results.
type AccessTokenRolesPage struct {
	pagination.LinkedPageBase
}

// IsEmpty determines whether or not a an AccessTokensPage contains any results.
func (r AccessTokenRolesPage) IsEmpty() (bool, error) {
	accessTokenRoles, err := ExtractAccessTokenRoles(r)
	return len(accessTokenRoles) == 0, err
}

// NextPageURL extracts the "next" link from the links section of the result.
func (r AccessTokenRolesPage) NextPageURL() (string, error) {
	var s struct {
		Links struct {
			Next     string `json:"next"`
			Previous string `json:"previous"`
		} `json:"links"`
	}
	err := r.ExtractInto(&s)
	if err != nil {
		return "", err
	}
	return s.Links.Next, err
}

// ExtractAccessTokenRoles returns a slice of AccessTokenRole contained in a
// single page of results.
func ExtractAccessTokenRoles(r pagination.Page) ([]AccessTokenRole, error) {
	var s struct {
		AccessTokenRoles []AccessTokenRole `json:"roles"`
	}
	err := (r.(AccessTokenRolesPage)).ExtractInto(&s)
	return s.AccessTokenRoles, err
}

type GetAccessTokenRoleResult struct {
	gophercloud.Result
}

// Extract interprets any GetAccessTokenRoleResult result as an AccessTokenRole.
func (r GetAccessTokenRoleResult) Extract() (*AccessTokenRole, error) {
	var s struct {
		AccessTokenRole *AccessTokenRole `json:"role"`
	}
	err := r.ExtractInto(&s)
	return s.AccessTokenRole, err
}

// OAuth1 is an OAuth1 object, returned in OAuth1 token result.
type OAuth1 struct {
	AccessTokenID string `json:"access_token_id"`
	ConsumerID    string `json:"consumer_id"`
}

// TokenExt represents an extension of the base token result.
type TokenExt struct {
	OAuth1 OAuth1 `json:"OS-OAUTH1"`
}
