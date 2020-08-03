/*
Package tokens provides information and interaction with the EC2 token API
resource for the OpenStack Identity service.

For more information, see:
https://docs.openstack.org/api-ref/identity/v2-ext/

Example to Create a Token From an EC2 access and secret keys

	var authOptions tokens.AuthOptionsBuilder
	authOptions = &ec2tokens.AuthOptions{
		Access: "a7f1e798b7c2417cba4a02de97dc3cdc",
		Secret: "18f4f6761ada4e3795fa5273c30349b9",
	}

	token, err := ec2tokens.Create(identityClient, authOptions).ExtractToken()
	if err != nil {
		panic(err)
	}

Example to auth a client using EC2 access and secret keys

	client, err := openstack.NewClient("http://localhost:5000/v3")
	if err != nil {
		panic(err)
	}

	var authOptions tokens.AuthOptionsBuilder
	authOptions = &ec2tokens.AuthOptions{
		Access:      "a7f1e798b7c2417cba4a02de97dc3cdc",
		Secret:      "18f4f6761ada4e3795fa5273c30349b9",
		AllowReauth: true,
	}

	err = openstack.AuthenticateV3(client, authOptions, gophercloud.EndpointOpts{})
	if err != nil {
		panic(err)
	}

*/
package ec2tokens
