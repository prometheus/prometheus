package gophercloud

/*
AuthOptions stores information needed to authenticate to an OpenStack Cloud.
You can populate one manually, or use a provider's AuthOptionsFromEnv() function
to read relevant information from the standard environment variables. Pass one
to a provider's AuthenticatedClient function to authenticate and obtain a
ProviderClient representing an active session on that provider.

Its fields are the union of those recognized by each identity implementation and
provider.

An example of manually providing authentication information:

  opts := gophercloud.AuthOptions{
    IdentityEndpoint: "https://openstack.example.com:5000/v2.0",
    Username: "{username}",
    Password: "{password}",
    TenantID: "{tenant_id}",
  }

  provider, err := openstack.AuthenticatedClient(opts)

An example of using AuthOptionsFromEnv(), where the environment variables can
be read from a file, such as a standard openrc file:

  opts, err := openstack.AuthOptionsFromEnv()
  provider, err := openstack.AuthenticatedClient(opts)
*/
type AuthOptions struct {
	// IdentityEndpoint specifies the HTTP endpoint that is required to work with
	// the Identity API of the appropriate version. While it's ultimately needed by
	// all of the identity services, it will often be populated by a provider-level
	// function.
	//
	// The IdentityEndpoint is typically referred to as the "auth_url" or
	// "OS_AUTH_URL" in the information provided by the cloud operator.
	IdentityEndpoint string `json:"-"`

	// Username is required if using Identity V2 API. Consult with your provider's
	// control panel to discover your account's username. In Identity V3, either
	// UserID or a combination of Username and DomainID or DomainName are needed.
	Username string `json:"username,omitempty"`
	UserID   string `json:"-"`

	Password string `json:"password,omitempty"`

	// Passcode is used in TOTP authentication method
	Passcode string `json:"passcode,omitempty"`

	// At most one of DomainID and DomainName must be provided if using Username
	// with Identity V3. Otherwise, either are optional.
	DomainID   string `json:"-"`
	DomainName string `json:"name,omitempty"`

	// The TenantID and TenantName fields are optional for the Identity V2 API.
	// The same fields are known as project_id and project_name in the Identity
	// V3 API, but are collected as TenantID and TenantName here in both cases.
	// Some providers allow you to specify a TenantName instead of the TenantId.
	// Some require both. Your provider's authentication policies will determine
	// how these fields influence authentication.
	// If DomainID or DomainName are provided, they will also apply to TenantName.
	// It is not currently possible to authenticate with Username and a Domain
	// and scope to a Project in a different Domain by using TenantName. To
	// accomplish that, the ProjectID will need to be provided as the TenantID
	// option.
	TenantID   string `json:"tenantId,omitempty"`
	TenantName string `json:"tenantName,omitempty"`

	// AllowReauth should be set to true if you grant permission for Gophercloud to
	// cache your credentials in memory, and to allow Gophercloud to attempt to
	// re-authenticate automatically if/when your token expires.  If you set it to
	// false, it will not cache these settings, but re-authentication will not be
	// possible.  This setting defaults to false.
	//
	// NOTE: The reauth function will try to re-authenticate endlessly if left
	// unchecked. The way to limit the number of attempts is to provide a custom
	// HTTP client to the provider client and provide a transport that implements
	// the RoundTripper interface and stores the number of failed retries. For an
	// example of this, see here:
	// https://github.com/rackspace/rack/blob/1.0.0/auth/clients.go#L311
	AllowReauth bool `json:"-"`

	// TokenID allows users to authenticate (possibly as another user) with an
	// authentication token ID.
	TokenID string `json:"-"`

	// Scope determines the scoping of the authentication request.
	Scope *AuthScope `json:"-"`

	// Authentication through Application Credentials requires supplying name, project and secret
	// For project we can use TenantID
	ApplicationCredentialID     string `json:"-"`
	ApplicationCredentialName   string `json:"-"`
	ApplicationCredentialSecret string `json:"-"`
}

// AuthScope allows a created token to be limited to a specific domain or project.
type AuthScope struct {
	ProjectID   string
	ProjectName string
	DomainID    string
	DomainName  string
	System      bool
}

// ToTokenV2CreateMap allows AuthOptions to satisfy the AuthOptionsBuilder
// interface in the v2 tokens package
func (opts AuthOptions) ToTokenV2CreateMap() (map[string]interface{}, error) {
	// Populate the request map.
	authMap := make(map[string]interface{})

	if opts.Username != "" {
		if opts.Password != "" {
			authMap["passwordCredentials"] = map[string]interface{}{
				"username": opts.Username,
				"password": opts.Password,
			}
		} else {
			return nil, ErrMissingInput{Argument: "Password"}
		}
	} else if opts.TokenID != "" {
		authMap["token"] = map[string]interface{}{
			"id": opts.TokenID,
		}
	} else {
		return nil, ErrMissingInput{Argument: "Username"}
	}

	if opts.TenantID != "" {
		authMap["tenantId"] = opts.TenantID
	}
	if opts.TenantName != "" {
		authMap["tenantName"] = opts.TenantName
	}

	return map[string]interface{}{"auth": authMap}, nil
}

// ToTokenV3CreateMap allows AuthOptions to satisfy the AuthOptionsBuilder
// interface in the v3 tokens package
func (opts *AuthOptions) ToTokenV3CreateMap(scope map[string]interface{}) (map[string]interface{}, error) {
	type domainReq struct {
		ID   *string `json:"id,omitempty"`
		Name *string `json:"name,omitempty"`
	}

	type projectReq struct {
		Domain *domainReq `json:"domain,omitempty"`
		Name   *string    `json:"name,omitempty"`
		ID     *string    `json:"id,omitempty"`
	}

	type userReq struct {
		ID       *string    `json:"id,omitempty"`
		Name     *string    `json:"name,omitempty"`
		Password *string    `json:"password,omitempty"`
		Passcode *string    `json:"passcode,omitempty"`
		Domain   *domainReq `json:"domain,omitempty"`
	}

	type passwordReq struct {
		User userReq `json:"user"`
	}

	type tokenReq struct {
		ID string `json:"id"`
	}

	type applicationCredentialReq struct {
		ID     *string  `json:"id,omitempty"`
		Name   *string  `json:"name,omitempty"`
		User   *userReq `json:"user,omitempty"`
		Secret *string  `json:"secret,omitempty"`
	}

	type totpReq struct {
		User *userReq `json:"user,omitempty"`
	}

	type identityReq struct {
		Methods               []string                  `json:"methods"`
		Password              *passwordReq              `json:"password,omitempty"`
		Token                 *tokenReq                 `json:"token,omitempty"`
		ApplicationCredential *applicationCredentialReq `json:"application_credential,omitempty"`
		TOTP                  *totpReq                  `json:"totp,omitempty"`
	}

	type authReq struct {
		Identity identityReq `json:"identity"`
	}

	type request struct {
		Auth authReq `json:"auth"`
	}

	// Populate the request structure based on the provided arguments. Create and return an error
	// if insufficient or incompatible information is present.
	var req request

	if opts.Password == "" && opts.Passcode == "" {
		if opts.TokenID != "" {
			// Because we aren't using password authentication, it's an error to also provide any of the user-based authentication
			// parameters.
			if opts.Username != "" {
				return nil, ErrUsernameWithToken{}
			}
			if opts.UserID != "" {
				return nil, ErrUserIDWithToken{}
			}
			if opts.DomainID != "" {
				return nil, ErrDomainIDWithToken{}
			}
			if opts.DomainName != "" {
				return nil, ErrDomainNameWithToken{}
			}

			// Configure the request for Token authentication.
			req.Auth.Identity.Methods = []string{"token"}
			req.Auth.Identity.Token = &tokenReq{
				ID: opts.TokenID,
			}

		} else if opts.ApplicationCredentialID != "" {
			// Configure the request for ApplicationCredentialID authentication.
			// https://github.com/openstack/keystoneauth/blob/stable/rocky/keystoneauth1/identity/v3/application_credential.py#L48-L67
			// There are three kinds of possible application_credential requests
			// 1. application_credential id + secret
			// 2. application_credential name + secret + user_id
			// 3. application_credential name + secret + username + domain_id / domain_name
			if opts.ApplicationCredentialSecret == "" {
				return nil, ErrAppCredMissingSecret{}
			}
			req.Auth.Identity.Methods = []string{"application_credential"}
			req.Auth.Identity.ApplicationCredential = &applicationCredentialReq{
				ID:     &opts.ApplicationCredentialID,
				Secret: &opts.ApplicationCredentialSecret,
			}
		} else if opts.ApplicationCredentialName != "" {
			if opts.ApplicationCredentialSecret == "" {
				return nil, ErrAppCredMissingSecret{}
			}

			var userRequest *userReq

			if opts.UserID != "" {
				// UserID could be used without the domain information
				userRequest = &userReq{
					ID: &opts.UserID,
				}
			}

			if userRequest == nil && opts.Username == "" {
				// Make sure that Username or UserID are provided
				return nil, ErrUsernameOrUserID{}
			}

			if userRequest == nil && opts.DomainID != "" {
				userRequest = &userReq{
					Name:   &opts.Username,
					Domain: &domainReq{ID: &opts.DomainID},
				}
			}

			if userRequest == nil && opts.DomainName != "" {
				userRequest = &userReq{
					Name:   &opts.Username,
					Domain: &domainReq{Name: &opts.DomainName},
				}
			}

			// Make sure that DomainID or DomainName are provided among Username
			if userRequest == nil {
				return nil, ErrDomainIDOrDomainName{}
			}

			req.Auth.Identity.Methods = []string{"application_credential"}
			req.Auth.Identity.ApplicationCredential = &applicationCredentialReq{
				Name:   &opts.ApplicationCredentialName,
				User:   userRequest,
				Secret: &opts.ApplicationCredentialSecret,
			}
		} else {
			// If no password or token ID or ApplicationCredential are available, authentication can't continue.
			return nil, ErrMissingPassword{}
		}
	} else {
		// Password authentication.
		if opts.Password != "" {
			req.Auth.Identity.Methods = append(req.Auth.Identity.Methods, "password")
		}

		// TOTP authentication.
		if opts.Passcode != "" {
			req.Auth.Identity.Methods = append(req.Auth.Identity.Methods, "totp")
		}

		// At least one of Username and UserID must be specified.
		if opts.Username == "" && opts.UserID == "" {
			return nil, ErrUsernameOrUserID{}
		}

		if opts.Username != "" {
			// If Username is provided, UserID may not be provided.
			if opts.UserID != "" {
				return nil, ErrUsernameOrUserID{}
			}

			// Either DomainID or DomainName must also be specified.
			if opts.DomainID == "" && opts.DomainName == "" {
				return nil, ErrDomainIDOrDomainName{}
			}

			if opts.DomainID != "" {
				if opts.DomainName != "" {
					return nil, ErrDomainIDOrDomainName{}
				}

				// Configure the request for Username and Password authentication with a DomainID.
				if opts.Password != "" {
					req.Auth.Identity.Password = &passwordReq{
						User: userReq{
							Name:     &opts.Username,
							Password: &opts.Password,
							Domain:   &domainReq{ID: &opts.DomainID},
						},
					}
				}
				if opts.Passcode != "" {
					req.Auth.Identity.TOTP = &totpReq{
						User: &userReq{
							Name:     &opts.Username,
							Passcode: &opts.Passcode,
							Domain:   &domainReq{ID: &opts.DomainID},
						},
					}
				}
			}

			if opts.DomainName != "" {
				// Configure the request for Username and Password authentication with a DomainName.
				if opts.Password != "" {
					req.Auth.Identity.Password = &passwordReq{
						User: userReq{
							Name:     &opts.Username,
							Password: &opts.Password,
							Domain:   &domainReq{Name: &opts.DomainName},
						},
					}
				}

				if opts.Passcode != "" {
					req.Auth.Identity.TOTP = &totpReq{
						User: &userReq{
							Name:     &opts.Username,
							Passcode: &opts.Passcode,
							Domain:   &domainReq{Name: &opts.DomainName},
						},
					}
				}
			}
		}

		if opts.UserID != "" {
			// If UserID is specified, neither DomainID nor DomainName may be.
			if opts.DomainID != "" {
				return nil, ErrDomainIDWithUserID{}
			}
			if opts.DomainName != "" {
				return nil, ErrDomainNameWithUserID{}
			}

			// Configure the request for UserID and Password authentication.
			if opts.Password != "" {
				req.Auth.Identity.Password = &passwordReq{
					User: userReq{
						ID:       &opts.UserID,
						Password: &opts.Password,
					},
				}
			}

			if opts.Passcode != "" {
				req.Auth.Identity.TOTP = &totpReq{
					User: &userReq{
						ID:       &opts.UserID,
						Passcode: &opts.Passcode,
					},
				}
			}
		}
	}

	b, err := BuildRequestBody(req, "")
	if err != nil {
		return nil, err
	}

	if len(scope) != 0 {
		b["auth"].(map[string]interface{})["scope"] = scope
	}

	return b, nil
}

// ToTokenV3ScopeMap builds a scope from AuthOptions and satisfies interface in
// the v3 tokens package.
func (opts *AuthOptions) ToTokenV3ScopeMap() (map[string]interface{}, error) {
	// For backwards compatibility.
	// If AuthOptions.Scope was not set, try to determine it.
	// This works well for common scenarios.
	if opts.Scope == nil {
		opts.Scope = new(AuthScope)
		if opts.TenantID != "" {
			opts.Scope.ProjectID = opts.TenantID
		} else {
			if opts.TenantName != "" {
				opts.Scope.ProjectName = opts.TenantName
				opts.Scope.DomainID = opts.DomainID
				opts.Scope.DomainName = opts.DomainName
			}
		}
	}

	if opts.Scope.System {
		return map[string]interface{}{
			"system": map[string]interface{}{
				"all": true,
			},
		}, nil
	}

	if opts.Scope.ProjectName != "" {
		// ProjectName provided: either DomainID or DomainName must also be supplied.
		// ProjectID may not be supplied.
		if opts.Scope.DomainID == "" && opts.Scope.DomainName == "" {
			return nil, ErrScopeDomainIDOrDomainName{}
		}
		if opts.Scope.ProjectID != "" {
			return nil, ErrScopeProjectIDOrProjectName{}
		}

		if opts.Scope.DomainID != "" {
			// ProjectName + DomainID
			return map[string]interface{}{
				"project": map[string]interface{}{
					"name":   &opts.Scope.ProjectName,
					"domain": map[string]interface{}{"id": &opts.Scope.DomainID},
				},
			}, nil
		}

		if opts.Scope.DomainName != "" {
			// ProjectName + DomainName
			return map[string]interface{}{
				"project": map[string]interface{}{
					"name":   &opts.Scope.ProjectName,
					"domain": map[string]interface{}{"name": &opts.Scope.DomainName},
				},
			}, nil
		}
	} else if opts.Scope.ProjectID != "" {
		// ProjectID provided. ProjectName, DomainID, and DomainName may not be provided.
		if opts.Scope.DomainID != "" {
			return nil, ErrScopeProjectIDAlone{}
		}
		if opts.Scope.DomainName != "" {
			return nil, ErrScopeProjectIDAlone{}
		}

		// ProjectID
		return map[string]interface{}{
			"project": map[string]interface{}{
				"id": &opts.Scope.ProjectID,
			},
		}, nil
	} else if opts.Scope.DomainID != "" {
		// DomainID provided. ProjectID, ProjectName, and DomainName may not be provided.
		if opts.Scope.DomainName != "" {
			return nil, ErrScopeDomainIDOrDomainName{}
		}

		// DomainID
		return map[string]interface{}{
			"domain": map[string]interface{}{
				"id": &opts.Scope.DomainID,
			},
		}, nil
	} else if opts.Scope.DomainName != "" {
		// DomainName
		return map[string]interface{}{
			"domain": map[string]interface{}{
				"name": &opts.Scope.DomainName,
			},
		}, nil
	}

	return nil, nil
}

func (opts AuthOptions) CanReauth() bool {
	if opts.Passcode != "" {
		// cannot reauth using TOTP passcode
		return false
	}

	return opts.AllowReauth
}

// ToTokenV3HeadersMap allows AuthOptions to satisfy the AuthOptionsBuilder
// interface in the v3 tokens package.
func (opts *AuthOptions) ToTokenV3HeadersMap(map[string]interface{}) (map[string]string, error) {
	return nil, nil
}
