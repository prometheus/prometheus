/*
Package oauth1 enables management of OpenStack OAuth1 tokens and Authentication.

Example to Create an OAuth1 Consumer

	createConsumerOpts := oauth1.CreateConsumerOpts{
		Description: "My consumer",
	}
	consumer, err := oauth1.CreateConsumer(identityClient, createConsumerOpts).Extract()
	if err != nil {
		panic(err)
	}

	// NOTE: Consumer secret is available only on create response
	fmt.Printf("Consumer: %+v\n", consumer)

Example to Request an unauthorized OAuth1 token

	requestTokenOpts := oauth1.RequestTokenOpts{
		OAuthConsumerKey:     consumer.ID,
		OAuthConsumerSecret:  consumer.Secret,
		OAuthSignatureMethod: oauth1.HMACSHA1,
		RequestedProjectID:   projectID,
	}
	requestToken, err := oauth1.RequestToken(identityClient, requestTokenOpts).Extract()
	if err != nil {
		panic(err)
	}

	// NOTE: Request token secret is available only on request response
	fmt.Printf("Request token: %+v\n", requestToken)

Example to Authorize an unauthorized OAuth1 token

	authorizeTokenOpts := oauth1.AuthorizeTokenOpts{
		Roles: []oauth1.Role{
			{Name: "member"},
		},
	}
	authToken, err := oauth1.AuthorizeToken(identityClient, requestToken.OAuthToken, authorizeTokenOpts).Extract()
	if err != nil {
		panic(err)
	}

	fmt.Printf("Verifier ID of the unauthorized Token: %+v\n", authToken.OAuthVerifier)

Example to Create an OAuth1 Access Token

	accessTokenOpts := oauth1.CreateAccessTokenOpts{
		OAuthConsumerKey:     consumer.ID,
		OAuthConsumerSecret:  consumer.Secret,
		OAuthToken:           requestToken.OAuthToken,
		OAuthTokenSecret:     requestToken.OAuthTokenSecret,
		OAuthVerifier:        authToken.OAuthVerifier,
		OAuthSignatureMethod: oauth1.HMACSHA1,
	}
	accessToken, err := oauth1.CreateAccessToken(identityClient, accessTokenOpts).Extract()
	if err != nil {
		panic(err)
	}

	// NOTE: Access token secret is available only on create response
	fmt.Printf("OAuth1 Access Token: %+v\n", accessToken)

Example to List User's OAuth1 Access Tokens

	allPages, err := oauth1.ListAccessTokens(identityClient, userID).AllPages()
	if err != nil {
		panic(err)
	}
	accessTokens, err := oauth1.ExtractAccessTokens(allPages)
	if err != nil {
		panic(err)
	}

	for _, accessToken := range accessTokens {
		fmt.Printf("Access Token: %+v\n", accessToken)
	}

Example to Authenticate a client using OAuth1 method

	client, err := openstack.NewClient("http://localhost:5000/v3")
	if err != nil {
		panic(err)
	}

	authOptions := &oauth1.AuthOptions{
		// consumer token, created earlier
		OAuthConsumerKey:    consumer.ID,
		OAuthConsumerSecret: consumer.Secret,
		// access token, created earlier
		OAuthToken:           accessToken.OAuthToken,
		OAuthTokenSecret:     accessToken.OAuthTokenSecret,
		OAuthSignatureMethod: oauth1.HMACSHA1,
	}
	err = openstack.AuthenticateV3(client, authOptions, gophercloud.EndpointOpts{})
	if err != nil {
		panic(err)
	}

Example to Create a Token using OAuth1 method

	var oauth1Token struct {
		tokens.Token
		oauth1.TokenExt
	}

	createOpts := &oauth1.AuthOptions{
		// consumer token, created earlier
		OAuthConsumerKey:    consumer.ID,
		OAuthConsumerSecret: consumer.Secret,
		// access token, created earlier
		OAuthToken:           accessToken.OAuthToken,
		OAuthTokenSecret:     accessToken.OAuthTokenSecret,
		OAuthSignatureMethod: oauth1.HMACSHA1,
	}
	err := tokens.Create(identityClient, createOpts).ExtractInto(&oauth1Token)
	if err != nil {
		panic(err)
	}

*/
package oauth1
