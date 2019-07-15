package session

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/processcreds"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/defaults"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/internal/shareddefaults"
)

// valid credential source values
const (
	credSourceEc2Metadata  = "Ec2InstanceMetadata"
	credSourceEnvironment  = "Environment"
	credSourceECSContainer = "EcsContainer"
)

func resolveCredentials(cfg *aws.Config,
	envCfg envConfig, sharedCfg sharedConfig,
	handlers request.Handlers,
	sessOpts Options,
) (*credentials.Credentials, error) {
	// Credentials from Assume Role with specific credentials source.
	if envCfg.EnableSharedConfig && len(sharedCfg.AssumeRole.CredentialSource) > 0 {
		return resolveCredsFromSource(cfg, envCfg, sharedCfg, handlers, sessOpts)
	}

	// Credentials from environment variables
	if len(envCfg.Creds.AccessKeyID) > 0 {
		return credentials.NewStaticCredentialsFromCreds(envCfg.Creds), nil
	}

	// Fallback to the "default" credential resolution chain.
	return resolveCredsFromProfile(cfg, envCfg, sharedCfg, handlers, sessOpts)
}

func resolveCredsFromProfile(cfg *aws.Config,
	envCfg envConfig, sharedCfg sharedConfig,
	handlers request.Handlers,
	sessOpts Options,
) (*credentials.Credentials, error) {

	if envCfg.EnableSharedConfig && len(sharedCfg.AssumeRole.RoleARN) > 0 && sharedCfg.AssumeRoleSource != nil {
		// Assume IAM role with credentials source from a different profile.
		cred, err := resolveCredsFromProfile(cfg, envCfg, *sharedCfg.AssumeRoleSource, handlers, sessOpts)
		if err != nil {
			return nil, err
		}

		cfgCp := *cfg
		cfgCp.Credentials = cred
		return credsFromAssumeRole(cfgCp, handlers, sharedCfg, sessOpts)

	} else if len(sharedCfg.Creds.AccessKeyID) > 0 {
		// Static Credentials from Shared Config/Credentials file.
		return credentials.NewStaticCredentialsFromCreds(
			sharedCfg.Creds,
		), nil

	} else if len(sharedCfg.CredentialProcess) > 0 {
		// Get credentials from CredentialProcess
		cred := processcreds.NewCredentials(sharedCfg.CredentialProcess)
		// if RoleARN is provided, so the obtained cred from the Credential Process to assume the role using RoleARN
		if len(sharedCfg.AssumeRole.RoleARN) > 0 {
			cfgCp := *cfg
			cfgCp.Credentials = cred
			return credsFromAssumeRole(cfgCp, handlers, sharedCfg, sessOpts)
		}
		return cred, nil
	} else if envCfg.EnableSharedConfig && len(sharedCfg.AssumeRole.CredentialSource) > 0 {
		// Assume IAM Role with specific credential source.
		return resolveCredsFromSource(cfg, envCfg, sharedCfg, handlers, sessOpts)
	}

	// Fallback to default credentials provider, include mock errors
	// for the credential chain so user can identify why credentials
	// failed to be retrieved.
	return credentials.NewCredentials(&credentials.ChainProvider{
		VerboseErrors: aws.BoolValue(cfg.CredentialsChainVerboseErrors),
		Providers: []credentials.Provider{
			&credProviderError{
				Err: awserr.New("EnvAccessKeyNotFound",
					"failed to find credentials in the environment.", nil),
			},
			&credProviderError{
				Err: awserr.New("SharedCredsLoad",
					fmt.Sprintf("failed to load profile, %s.", envCfg.Profile), nil),
			},
			defaults.RemoteCredProvider(*cfg, handlers),
		},
	}), nil
}

func resolveCredsFromSource(cfg *aws.Config,
	envCfg envConfig, sharedCfg sharedConfig,
	handlers request.Handlers,
	sessOpts Options,
) (*credentials.Credentials, error) {
	// if both credential_source and source_profile have been set, return an
	// error as this is undefined behavior. Only one can be used at a time
	// within a profile.
	if len(sharedCfg.AssumeRole.SourceProfile) > 0 {
		return nil, ErrSharedConfigSourceCollision
	}

	cfgCp := *cfg
	switch sharedCfg.AssumeRole.CredentialSource {
	case credSourceEc2Metadata:
		p := defaults.RemoteCredProvider(cfgCp, handlers)
		cfgCp.Credentials = credentials.NewCredentials(p)

	case credSourceEnvironment:
		cfgCp.Credentials = credentials.NewStaticCredentialsFromCreds(envCfg.Creds)

	case credSourceECSContainer:
		if len(os.Getenv(shareddefaults.ECSCredsProviderEnvVar)) == 0 {
			return nil, ErrSharedConfigECSContainerEnvVarEmpty
		}

		p := defaults.RemoteCredProvider(cfgCp, handlers)
		cfgCp.Credentials = credentials.NewCredentials(p)

	default:
		return nil, ErrSharedConfigInvalidCredSource
	}

	return credsFromAssumeRole(cfgCp, handlers, sharedCfg, sessOpts)
}

func credsFromAssumeRole(cfg aws.Config,
	handlers request.Handlers,
	sharedCfg sharedConfig,
	sessOpts Options,
) (*credentials.Credentials, error) {
	if len(sharedCfg.AssumeRole.MFASerial) > 0 && sessOpts.AssumeRoleTokenProvider == nil {
		// AssumeRole Token provider is required if doing Assume Role
		// with MFA.
		return nil, AssumeRoleTokenProviderNotSetError{}
	}

	return stscreds.NewCredentials(
		&Session{
			Config:   &cfg,
			Handlers: handlers.Copy(),
		},
		sharedCfg.AssumeRole.RoleARN,
		func(opt *stscreds.AssumeRoleProvider) {
			opt.RoleSessionName = sharedCfg.AssumeRole.RoleSessionName
			opt.Duration = sessOpts.AssumeRoleDuration

			// Assume role with external ID
			if len(sharedCfg.AssumeRole.ExternalID) > 0 {
				opt.ExternalID = aws.String(sharedCfg.AssumeRole.ExternalID)
			}

			// Assume role with MFA
			if len(sharedCfg.AssumeRole.MFASerial) > 0 {
				opt.SerialNumber = aws.String(sharedCfg.AssumeRole.MFASerial)
				opt.TokenProvider = sessOpts.AssumeRoleTokenProvider
			}
		},
	), nil
}

// AssumeRoleTokenProviderNotSetError is an error returned when creating a session when the
// MFAToken option is not set when shared config is configured load assume a
// role with an MFA token.
type AssumeRoleTokenProviderNotSetError struct{}

// Code is the short id of the error.
func (e AssumeRoleTokenProviderNotSetError) Code() string {
	return "AssumeRoleTokenProviderNotSetError"
}

// Message is the description of the error
func (e AssumeRoleTokenProviderNotSetError) Message() string {
	return fmt.Sprintf("assume role with MFA enabled, but AssumeRoleTokenProvider session option not set.")
}

// OrigErr is the underlying error that caused the failure.
func (e AssumeRoleTokenProviderNotSetError) OrigErr() error {
	return nil
}

// Error satisfies the error interface.
func (e AssumeRoleTokenProviderNotSetError) Error() string {
	return awserr.SprintError(e.Code(), e.Message(), "", nil)
}

type credProviderError struct {
	Err error
}

var emptyCreds = credentials.Value{}

func (c credProviderError) Retrieve() (credentials.Value, error) {
	return credentials.Value{}, c.Err
}
func (c credProviderError) IsExpired() bool {
	return true
}
