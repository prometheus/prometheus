/*
Package session provides configuration for the SDK's service clients. Sessions
can be shared across service clients that share the same base configuration.

Sessions are safe to use concurrently as long as the Session is not being
modified. Sessions should be cached when possible, because creating a new
Session will load all configuration values from the environment, and config
files each time the Session is created. Sharing the Session value across all of
your service clients will ensure the configuration is loaded the fewest number
of times possible.

Sessions options from Shared Config

By default NewSession will only load credentials from the shared credentials
file (~/.aws/credentials). If the AWS_SDK_LOAD_CONFIG environment variable is
set to a truthy value the Session will be created from the configuration
values from the shared config (~/.aws/config) and shared credentials
(~/.aws/credentials) files. Using the NewSessionWithOptions with
SharedConfigState set to SharedConfigEnable will create the session as if the
AWS_SDK_LOAD_CONFIG environment variable was set.

Credential and config loading order

The Session will attempt to load configuration and credentials from the
environment, configuration files, and other credential sources. The order
configuration is loaded in is:

  * Environment Variables
  * Shared Credentials file
  * Shared Configuration file (if SharedConfig is enabled)
  * EC2 Instance Metadata (credentials only)

The Environment variables for credentials will have precedence over shared
config even if SharedConfig is enabled. To override this behavior, and use
shared config credentials instead specify the session.Options.Profile, (e.g.
when using credential_source=Environment to assume a role).

  sess, err := session.NewSessionWithOptions(session.Options{
	  Profile: "myProfile",
  })

Creating Sessions

Creating a Session without additional options will load credentials region, and
profile loaded from the environment and shared config automatically. See,
"Environment Variables" section for information on environment variables used
by Session.

	// Create Session
	sess, err := session.NewSession()


When creating Sessions optional aws.Config values can be passed in that will
override the default, or loaded, config values the Session is being created
with. This allows you to provide additional, or case based, configuration
as needed.

	// Create a Session with a custom region
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-west-2"),
	})

Use NewSessionWithOptions to provide additional configuration driving how the
Session's configuration will be loaded. Such as, specifying shared config
profile, or override the shared config state,  (AWS_SDK_LOAD_CONFIG).

	// Equivalent to session.NewSession()
	sess, err := session.NewSessionWithOptions(session.Options{
		// Options
	})

	sess, err := session.NewSessionWithOptions(session.Options{
		// Specify profile to load for the session's config
		Profile: "profile_name",

		// Provide SDK Config options, such as Region.
		Config: aws.Config{
			Region: aws.String("us-west-2"),
		},

		// Force enable Shared Config support
		SharedConfigState: session.SharedConfigEnable,
	})

Adding Handlers

You can add handlers to a session to decorate API operation, (e.g. adding HTTP
headers). All clients that use the Session receive a copy of the Session's
handlers. For example, the following request handler added to the Session logs
every requests made.

	// Create a session, and add additional handlers for all service
	// clients created with the Session to inherit. Adds logging handler.
	sess := session.Must(session.NewSession())

	sess.Handlers.Send.PushFront(func(r *request.Request) {
		// Log every request made and its payload
		logger.Printf("Request: %s/%s, Params: %s",
			r.ClientInfo.ServiceName, r.Operation, r.Params)
	})

Shared Config Fields

By default the SDK will only load the shared credentials file's
(~/.aws/credentials) credentials values, and all other config is provided by
the environment variables, SDK defaults, and user provided aws.Config values.

If the AWS_SDK_LOAD_CONFIG environment variable is set, or SharedConfigEnable
option is used to create the Session the full shared config values will be
loaded. This includes credentials, region, and support for assume role. In
addition the Session will load its configuration from both the shared config
file (~/.aws/config) and shared credentials file (~/.aws/credentials). Both
files have the same format.

If both config files are present the configuration from both files will be
read. The Session will be created from configuration values from the shared
credentials file (~/.aws/credentials) over those in the shared config file
(~/.aws/config).

Credentials are the values the SDK uses to authenticating requests with AWS
Services. When specified in a file, both aws_access_key_id and
aws_secret_access_key must be provided together in the same file to be
considered valid. They will be ignored if both are not present.
aws_session_token is an optional field that can be provided in addition to the
other two fields.

	aws_access_key_id = AKID
	aws_secret_access_key = SECRET
	aws_session_token = TOKEN

	; region only supported if SharedConfigEnabled.
	region = us-east-1

Assume Role configuration

The role_arn field allows you to configure the SDK to assume an IAM role using
a set of credentials from another source. Such as when paired with static
credentials, "profile_source", "credential_process", or "credential_source"
fields. If "role_arn" is provided, a source of credentials must also be
specified, such as "source_profile", "credential_source", or
"credential_process".

	role_arn = arn:aws:iam::<account_number>:role/<role_name>
	source_profile = profile_with_creds
	external_id = 1234
	mfa_serial = <serial or mfa arn>
	role_session_name = session_name


The SDK supports assuming a role with MFA token. If "mfa_serial" is set, you
must also set the Session Option.AssumeRoleTokenProvider. The Session will fail
to load if the AssumeRoleTokenProvider is not specified.

    sess := session.Must(session.NewSessionWithOptions(session.Options{
        AssumeRoleTokenProvider: stscreds.StdinTokenProvider,
    }))

To setup Assume Role outside of a session see the stscreds.AssumeRoleProvider
documentation.

Environment Variables

When a Session is created several environment variables can be set to adjust
how the SDK functions, and what configuration data it loads when creating
Sessions. All environment values are optional, but some values like credentials
require multiple of the values to set or the partial values will be ignored.
All environment variable values are strings unless otherwise noted.

Environment configuration values. If set both Access Key ID and Secret Access
Key must be provided. Session Token and optionally also be provided, but is
not required.

	# Access Key ID
	AWS_ACCESS_KEY_ID=AKID
	AWS_ACCESS_KEY=AKID # only read if AWS_ACCESS_KEY_ID is not set.

	# Secret Access Key
	AWS_SECRET_ACCESS_KEY=SECRET
	AWS_SECRET_KEY=SECRET=SECRET # only read if AWS_SECRET_ACCESS_KEY is not set.

	# Session Token
	AWS_SESSION_TOKEN=TOKEN

Region value will instruct the SDK where to make service API requests to. If is
not provided in the environment the region must be provided before a service
client request is made.

	AWS_REGION=us-east-1

	# AWS_DEFAULT_REGION is only read if AWS_SDK_LOAD_CONFIG is also set,
	# and AWS_REGION is not also set.
	AWS_DEFAULT_REGION=us-east-1

Profile name the SDK should load use when loading shared config from the
configuration files. If not provided "default" will be used as the profile name.

	AWS_PROFILE=my_profile

	# AWS_DEFAULT_PROFILE is only read if AWS_SDK_LOAD_CONFIG is also set,
	# and AWS_PROFILE is not also set.
	AWS_DEFAULT_PROFILE=my_profile

SDK load config instructs the SDK to load the shared config in addition to
shared credentials. This also expands the configuration loaded so the shared
credentials will have parity with the shared config file. This also enables
Region and Profile support for the AWS_DEFAULT_REGION and AWS_DEFAULT_PROFILE
env values as well.

	AWS_SDK_LOAD_CONFIG=1

Shared credentials file path can be set to instruct the SDK to use an alternative
file for the shared credentials. If not set the file will be loaded from
$HOME/.aws/credentials on Linux/Unix based systems, and
%USERPROFILE%\.aws\credentials on Windows.

	AWS_SHARED_CREDENTIALS_FILE=$HOME/my_shared_credentials

Shared config file path can be set to instruct the SDK to use an alternative
file for the shared config. If not set the file will be loaded from
$HOME/.aws/config on Linux/Unix based systems, and
%USERPROFILE%\.aws\config on Windows.

	AWS_CONFIG_FILE=$HOME/my_shared_config

Path to a custom Credentials Authority (CA) bundle PEM file that the SDK
will use instead of the default system's root CA bundle. Use this only
if you want to replace the CA bundle the SDK uses for TLS requests.

	AWS_CA_BUNDLE=$HOME/my_custom_ca_bundle

Enabling this option will attempt to merge the Transport into the SDK's HTTP
client. If the client's Transport is not a http.Transport an error will be
returned. If the Transport's TLS config is set this option will cause the SDK
to overwrite the Transport's TLS config's  RootCAs value. If the CA bundle file
contains multiple certificates all of them will be loaded.

The Session option CustomCABundle is also available when creating sessions
to also enable this feature. CustomCABundle session option field has priority
over the AWS_CA_BUNDLE environment variable, and will be used if both are set.

Setting a custom HTTPClient in the aws.Config options will override this setting.
To use this option and custom HTTP client, the HTTP client needs to be provided
when creating the session. Not the service client.
*/
package session
