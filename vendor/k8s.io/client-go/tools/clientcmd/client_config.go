/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package clientcmd

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"strings"

	"github.com/golang/glog"
	"github.com/imdario/mergo"

	"k8s.io/client-go/pkg/api"
	restclient "k8s.io/client-go/rest"
	clientauth "k8s.io/client-go/tools/auth"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var (
	// ClusterDefaults has the same behavior as the old EnvVar and DefaultCluster fields
	// DEPRECATED will be replaced
	ClusterDefaults = clientcmdapi.Cluster{Server: getDefaultServer()}
	// DefaultClientConfig represents the legacy behavior of this package for defaulting
	// DEPRECATED will be replace
	DefaultClientConfig = DirectClientConfig{*clientcmdapi.NewConfig(), "", &ConfigOverrides{
		ClusterDefaults: ClusterDefaults,
	}, nil, NewDefaultClientConfigLoadingRules(), promptedCredentials{}}
)

// getDefaultServer returns a default setting for DefaultClientConfig
// DEPRECATED
func getDefaultServer() string {
	if server := os.Getenv("KUBERNETES_MASTER"); len(server) > 0 {
		return server
	}
	return "http://localhost:8080"
}

// ClientConfig is used to make it easy to get an api server client
type ClientConfig interface {
	// RawConfig returns the merged result of all overrides
	RawConfig() (clientcmdapi.Config, error)
	// ClientConfig returns a complete client config
	ClientConfig() (*restclient.Config, error)
	// Namespace returns the namespace resulting from the merged
	// result of all overrides and a boolean indicating if it was
	// overridden
	Namespace() (string, bool, error)
	// ConfigAccess returns the rules for loading/persisting the config.
	ConfigAccess() ConfigAccess
}

type PersistAuthProviderConfigForUser func(user string) restclient.AuthProviderConfigPersister

type promptedCredentials struct {
	username string
	password string
}

// DirectClientConfig is a ClientConfig interface that is backed by a clientcmdapi.Config, options overrides, and an optional fallbackReader for auth information
type DirectClientConfig struct {
	config         clientcmdapi.Config
	contextName    string
	overrides      *ConfigOverrides
	fallbackReader io.Reader
	configAccess   ConfigAccess
	// promptedCredentials store the credentials input by the user
	promptedCredentials promptedCredentials
}

// NewDefaultClientConfig creates a DirectClientConfig using the config.CurrentContext as the context name
func NewDefaultClientConfig(config clientcmdapi.Config, overrides *ConfigOverrides) ClientConfig {
	return &DirectClientConfig{config, config.CurrentContext, overrides, nil, NewDefaultClientConfigLoadingRules(), promptedCredentials{}}
}

// NewNonInteractiveClientConfig creates a DirectClientConfig using the passed context name and does not have a fallback reader for auth information
func NewNonInteractiveClientConfig(config clientcmdapi.Config, contextName string, overrides *ConfigOverrides, configAccess ConfigAccess) ClientConfig {
	return &DirectClientConfig{config, contextName, overrides, nil, configAccess, promptedCredentials{}}
}

// NewInteractiveClientConfig creates a DirectClientConfig using the passed context name and a reader in case auth information is not provided via files or flags
func NewInteractiveClientConfig(config clientcmdapi.Config, contextName string, overrides *ConfigOverrides, fallbackReader io.Reader, configAccess ConfigAccess) ClientConfig {
	return &DirectClientConfig{config, contextName, overrides, fallbackReader, configAccess, promptedCredentials{}}
}

func (config *DirectClientConfig) RawConfig() (clientcmdapi.Config, error) {
	return config.config, nil
}

// ClientConfig implements ClientConfig
func (config *DirectClientConfig) ClientConfig() (*restclient.Config, error) {
	// check that getAuthInfo, getContext, and getCluster do not return an error.
	// Do this before checking if the curent config is usable in the event that an
	// AuthInfo, Context, or Cluster config with user-defined names are not found.
	// This provides a user with the immediate cause for error if one is found
	configAuthInfo, err := config.getAuthInfo()
	if err != nil {
		return nil, err
	}

	_, err = config.getContext()
	if err != nil {
		return nil, err
	}

	configClusterInfo, err := config.getCluster()
	if err != nil {
		return nil, err
	}

	if err := config.ConfirmUsable(); err != nil {
		return nil, err
	}

	clientConfig := &restclient.Config{}
	clientConfig.Host = configClusterInfo.Server

	if len(config.overrides.Timeout) > 0 {
		timeout, err := ParseTimeout(config.overrides.Timeout)
		if err != nil {
			return nil, err
		}
		clientConfig.Timeout = timeout
	}

	if u, err := url.ParseRequestURI(clientConfig.Host); err == nil && u.Opaque == "" && len(u.Path) > 1 {
		u.RawQuery = ""
		u.Fragment = ""
		clientConfig.Host = u.String()
	}
	if len(configAuthInfo.Impersonate) > 0 {
		clientConfig.Impersonate = restclient.ImpersonationConfig{UserName: configAuthInfo.Impersonate}
	}

	// only try to read the auth information if we are secure
	if restclient.IsConfigTransportTLS(*clientConfig) {
		var err error

		// mergo is a first write wins for map value and a last writing wins for interface values
		// NOTE: This behavior changed with https://github.com/imdario/mergo/commit/d304790b2ed594794496464fadd89d2bb266600a.
		//       Our mergo.Merge version is older than this change.
		var persister restclient.AuthProviderConfigPersister
		if config.configAccess != nil {
			authInfoName, _ := config.getAuthInfoName()
			persister = PersisterForUser(config.configAccess, authInfoName)
		}
		userAuthPartialConfig, err := config.getUserIdentificationPartialConfig(configAuthInfo, config.fallbackReader, persister)
		if err != nil {
			return nil, err
		}
		mergo.Merge(clientConfig, userAuthPartialConfig)

		serverAuthPartialConfig, err := getServerIdentificationPartialConfig(configAuthInfo, configClusterInfo)
		if err != nil {
			return nil, err
		}
		mergo.Merge(clientConfig, serverAuthPartialConfig)
	}

	return clientConfig, nil
}

// clientauth.Info object contain both user identification and server identification.  We want different precedence orders for
// both, so we have to split the objects and merge them separately
// we want this order of precedence for the server identification
// 1.  configClusterInfo (the final result of command line flags and merged .kubeconfig files)
// 2.  configAuthInfo.auth-path (this file can contain information that conflicts with #1, and we want #1 to win the priority)
// 3.  load the ~/.kubernetes_auth file as a default
func getServerIdentificationPartialConfig(configAuthInfo clientcmdapi.AuthInfo, configClusterInfo clientcmdapi.Cluster) (*restclient.Config, error) {
	mergedConfig := &restclient.Config{}

	// configClusterInfo holds the information identify the server provided by .kubeconfig
	configClientConfig := &restclient.Config{}
	configClientConfig.CAFile = configClusterInfo.CertificateAuthority
	configClientConfig.CAData = configClusterInfo.CertificateAuthorityData
	configClientConfig.Insecure = configClusterInfo.InsecureSkipTLSVerify
	mergo.Merge(mergedConfig, configClientConfig)

	return mergedConfig, nil
}

// clientauth.Info object contain both user identification and server identification.  We want different precedence orders for
// both, so we have to split the objects and merge them separately
// we want this order of precedence for user identifcation
// 1.  configAuthInfo minus auth-path (the final result of command line flags and merged .kubeconfig files)
// 2.  configAuthInfo.auth-path (this file can contain information that conflicts with #1, and we want #1 to win the priority)
// 3.  if there is not enough information to idenfity the user, load try the ~/.kubernetes_auth file
// 4.  if there is not enough information to identify the user, prompt if possible
func (config *DirectClientConfig) getUserIdentificationPartialConfig(configAuthInfo clientcmdapi.AuthInfo, fallbackReader io.Reader, persistAuthConfig restclient.AuthProviderConfigPersister) (*restclient.Config, error) {
	mergedConfig := &restclient.Config{}

	// blindly overwrite existing values based on precedence
	if len(configAuthInfo.Token) > 0 {
		mergedConfig.BearerToken = configAuthInfo.Token
	} else if len(configAuthInfo.TokenFile) > 0 {
		tokenBytes, err := ioutil.ReadFile(configAuthInfo.TokenFile)
		if err != nil {
			return nil, err
		}
		mergedConfig.BearerToken = string(tokenBytes)
	}
	if len(configAuthInfo.Impersonate) > 0 {
		mergedConfig.Impersonate = restclient.ImpersonationConfig{UserName: configAuthInfo.Impersonate}
	}
	if len(configAuthInfo.ClientCertificate) > 0 || len(configAuthInfo.ClientCertificateData) > 0 {
		mergedConfig.CertFile = configAuthInfo.ClientCertificate
		mergedConfig.CertData = configAuthInfo.ClientCertificateData
		mergedConfig.KeyFile = configAuthInfo.ClientKey
		mergedConfig.KeyData = configAuthInfo.ClientKeyData
	}
	if len(configAuthInfo.Username) > 0 || len(configAuthInfo.Password) > 0 {
		mergedConfig.Username = configAuthInfo.Username
		mergedConfig.Password = configAuthInfo.Password
	}
	if configAuthInfo.AuthProvider != nil {
		mergedConfig.AuthProvider = configAuthInfo.AuthProvider
		mergedConfig.AuthConfigPersister = persistAuthConfig
	}

	// if there still isn't enough information to authenticate the user, try prompting
	if !canIdentifyUser(*mergedConfig) && (fallbackReader != nil) {
		if len(config.promptedCredentials.username) > 0 && len(config.promptedCredentials.password) > 0 {
			mergedConfig.Username = config.promptedCredentials.username
			mergedConfig.Password = config.promptedCredentials.password
			return mergedConfig, nil
		}
		prompter := NewPromptingAuthLoader(fallbackReader)
		promptedAuthInfo, err := prompter.Prompt()
		if err != nil {
			return nil, err
		}
		promptedConfig := makeUserIdentificationConfig(*promptedAuthInfo)
		previouslyMergedConfig := mergedConfig
		mergedConfig = &restclient.Config{}
		mergo.Merge(mergedConfig, promptedConfig)
		mergo.Merge(mergedConfig, previouslyMergedConfig)
		config.promptedCredentials.username = mergedConfig.Username
		config.promptedCredentials.password = mergedConfig.Password
	}

	return mergedConfig, nil
}

// makeUserIdentificationFieldsConfig returns a client.Config capable of being merged using mergo for only user identification information
func makeUserIdentificationConfig(info clientauth.Info) *restclient.Config {
	config := &restclient.Config{}
	config.Username = info.User
	config.Password = info.Password
	config.CertFile = info.CertFile
	config.KeyFile = info.KeyFile
	config.BearerToken = info.BearerToken
	return config
}

// makeUserIdentificationFieldsConfig returns a client.Config capable of being merged using mergo for only server identification information
func makeServerIdentificationConfig(info clientauth.Info) restclient.Config {
	config := restclient.Config{}
	config.CAFile = info.CAFile
	if info.Insecure != nil {
		config.Insecure = *info.Insecure
	}
	return config
}

func canIdentifyUser(config restclient.Config) bool {
	return len(config.Username) > 0 ||
		(len(config.CertFile) > 0 || len(config.CertData) > 0) ||
		len(config.BearerToken) > 0 ||
		config.AuthProvider != nil
}

// Namespace implements ClientConfig
func (config *DirectClientConfig) Namespace() (string, bool, error) {
	if err := config.ConfirmUsable(); err != nil {
		return "", false, err
	}

	configContext, err := config.getContext()
	if err != nil {
		return "", false, err
	}

	if len(configContext.Namespace) == 0 {
		return api.NamespaceDefault, false, nil
	}

	overridden := false
	if config.overrides != nil && config.overrides.Context.Namespace != "" {
		overridden = true
	}
	return configContext.Namespace, overridden, nil
}

// ConfigAccess implements ClientConfig
func (config *DirectClientConfig) ConfigAccess() ConfigAccess {
	return config.configAccess
}

// ConfirmUsable looks a particular context and determines if that particular part of the config is useable.  There might still be errors in the config,
// but no errors in the sections requested or referenced.  It does not return early so that it can find as many errors as possible.
func (config *DirectClientConfig) ConfirmUsable() error {
	validationErrors := make([]error, 0)

	var contextName string
	if len(config.contextName) != 0 {
		contextName = config.contextName
	} else {
		contextName = config.config.CurrentContext
	}

	if len(contextName) > 0 {
		_, exists := config.config.Contexts[contextName]
		if !exists {
			validationErrors = append(validationErrors, &errContextNotFound{contextName})
		}
	}

	authInfoName, _ := config.getAuthInfoName()
	authInfo, _ := config.getAuthInfo()
	validationErrors = append(validationErrors, validateAuthInfo(authInfoName, authInfo)...)
	clusterName, _ := config.getClusterName()
	cluster, _ := config.getCluster()
	validationErrors = append(validationErrors, validateClusterInfo(clusterName, cluster)...)
	// when direct client config is specified, and our only error is that no server is defined, we should
	// return a standard "no config" error
	if len(validationErrors) == 1 && validationErrors[0] == ErrEmptyCluster {
		return newErrConfigurationInvalid([]error{ErrEmptyConfig})
	}
	return newErrConfigurationInvalid(validationErrors)
}

// getContextName returns the default, or user-set context name, and a boolean that indicates
// whether the default context name has been overwritten by a user-set flag, or left as its default value
func (config *DirectClientConfig) getContextName() (string, bool) {
	if len(config.overrides.CurrentContext) != 0 {
		return config.overrides.CurrentContext, true
	}
	if len(config.contextName) != 0 {
		return config.contextName, false
	}

	return config.config.CurrentContext, false
}

// getAuthInfoName returns a string containing the current authinfo name for the current context,
// and a boolean indicating  whether the default authInfo name is overwritten by a user-set flag, or
// left as its default value
func (config *DirectClientConfig) getAuthInfoName() (string, bool) {
	if len(config.overrides.Context.AuthInfo) != 0 {
		return config.overrides.Context.AuthInfo, true
	}
	context, _ := config.getContext()
	return context.AuthInfo, false
}

// getClusterName returns a string containing the default, or user-set cluster name, and a boolean
// indicating whether the default clusterName has been overwritten by a user-set flag, or left as
// its default value
func (config *DirectClientConfig) getClusterName() (string, bool) {
	if len(config.overrides.Context.Cluster) != 0 {
		return config.overrides.Context.Cluster, true
	}
	context, _ := config.getContext()
	return context.Cluster, false
}

// getContext returns the clientcmdapi.Context, or an error if a required context is not found.
func (config *DirectClientConfig) getContext() (clientcmdapi.Context, error) {
	contexts := config.config.Contexts
	contextName, required := config.getContextName()

	mergedContext := clientcmdapi.NewContext()
	if configContext, exists := contexts[contextName]; exists {
		mergo.Merge(mergedContext, configContext)
	} else if required {
		return clientcmdapi.Context{}, fmt.Errorf("context %q does not exist", contextName)
	}
	mergo.Merge(mergedContext, config.overrides.Context)

	return *mergedContext, nil
}

// getAuthInfo returns the clientcmdapi.AuthInfo, or an error if a required auth info is not found.
func (config *DirectClientConfig) getAuthInfo() (clientcmdapi.AuthInfo, error) {
	authInfos := config.config.AuthInfos
	authInfoName, required := config.getAuthInfoName()

	mergedAuthInfo := clientcmdapi.NewAuthInfo()
	if configAuthInfo, exists := authInfos[authInfoName]; exists {
		mergo.Merge(mergedAuthInfo, configAuthInfo)
	} else if required {
		return clientcmdapi.AuthInfo{}, fmt.Errorf("auth info %q does not exist", authInfoName)
	}
	mergo.Merge(mergedAuthInfo, config.overrides.AuthInfo)

	return *mergedAuthInfo, nil
}

// getCluster returns the clientcmdapi.Cluster, or an error if a required cluster is not found.
func (config *DirectClientConfig) getCluster() (clientcmdapi.Cluster, error) {
	clusterInfos := config.config.Clusters
	clusterInfoName, required := config.getClusterName()

	mergedClusterInfo := clientcmdapi.NewCluster()
	mergo.Merge(mergedClusterInfo, config.overrides.ClusterDefaults)
	if configClusterInfo, exists := clusterInfos[clusterInfoName]; exists {
		mergo.Merge(mergedClusterInfo, configClusterInfo)
	} else if required {
		return clientcmdapi.Cluster{}, fmt.Errorf("cluster %q does not exist", clusterInfoName)
	}
	mergo.Merge(mergedClusterInfo, config.overrides.ClusterInfo)
	// An override of --insecure-skip-tls-verify=true and no accompanying CA/CA data should clear already-set CA/CA data
	// otherwise, a kubeconfig containing a CA reference would return an error that "CA and insecure-skip-tls-verify couldn't both be set"
	caLen := len(config.overrides.ClusterInfo.CertificateAuthority)
	caDataLen := len(config.overrides.ClusterInfo.CertificateAuthorityData)
	if config.overrides.ClusterInfo.InsecureSkipTLSVerify && caLen == 0 && caDataLen == 0 {
		mergedClusterInfo.CertificateAuthority = ""
		mergedClusterInfo.CertificateAuthorityData = nil
	}

	return *mergedClusterInfo, nil
}

// inClusterClientConfig makes a config that will work from within a kubernetes cluster container environment.
// Can take options overrides for flags explicitly provided to the command inside the cluster container.
type inClusterClientConfig struct {
	overrides               *ConfigOverrides
	inClusterConfigProvider func() (*restclient.Config, error)
}

var _ ClientConfig = &inClusterClientConfig{}

func (config *inClusterClientConfig) RawConfig() (clientcmdapi.Config, error) {
	return clientcmdapi.Config{}, fmt.Errorf("inCluster environment config doesn't support multiple clusters")
}

func (config *inClusterClientConfig) ClientConfig() (*restclient.Config, error) {
	if config.inClusterConfigProvider == nil {
		config.inClusterConfigProvider = restclient.InClusterConfig
	}

	icc, err := config.inClusterConfigProvider()
	if err != nil {
		return nil, err
	}

	// in-cluster configs only takes a host, token, or CA file
	// if any of them were individually provided, ovewrite anything else
	if config.overrides != nil {
		if server := config.overrides.ClusterInfo.Server; len(server) > 0 {
			icc.Host = server
		}
		if token := config.overrides.AuthInfo.Token; len(token) > 0 {
			icc.BearerToken = token
		}
		if certificateAuthorityFile := config.overrides.ClusterInfo.CertificateAuthority; len(certificateAuthorityFile) > 0 {
			icc.TLSClientConfig.CAFile = certificateAuthorityFile
		}
	}

	return icc, err
}

func (config *inClusterClientConfig) Namespace() (string, bool, error) {
	// This way assumes you've set the POD_NAMESPACE environment variable using the downward API.
	// This check has to be done first for backwards compatibility with the way InClusterConfig was originally set up
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns, true, nil
	}

	// Fall back to the namespace associated with the service account token, if available
	if data, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		if ns := strings.TrimSpace(string(data)); len(ns) > 0 {
			return ns, true, nil
		}
	}

	return "default", false, nil
}

func (config *inClusterClientConfig) ConfigAccess() ConfigAccess {
	return NewDefaultClientConfigLoadingRules()
}

// Possible returns true if loading an inside-kubernetes-cluster is possible.
func (config *inClusterClientConfig) Possible() bool {
	fi, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token")
	return os.Getenv("KUBERNETES_SERVICE_HOST") != "" &&
		os.Getenv("KUBERNETES_SERVICE_PORT") != "" &&
		err == nil && !fi.IsDir()
}

// BuildConfigFromFlags is a helper function that builds configs from a master
// url or a kubeconfig filepath. These are passed in as command line flags for cluster
// components. Warnings should reflect this usage. If neither masterUrl or kubeconfigPath
// are passed in we fallback to inClusterConfig. If inClusterConfig fails, we fallback
// to the default config.
func BuildConfigFromFlags(masterUrl, kubeconfigPath string) (*restclient.Config, error) {
	if kubeconfigPath == "" && masterUrl == "" {
		glog.Warningf("Neither --kubeconfig nor --master was specified.  Using the inClusterConfig.  This might not work.")
		kubeconfig, err := restclient.InClusterConfig()
		if err == nil {
			return kubeconfig, nil
		}
		glog.Warning("error creating inClusterConfig, falling back to default config: ", err)
	}
	return NewNonInteractiveDeferredLoadingClientConfig(
		&ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: masterUrl}}).ClientConfig()
}

// BuildConfigFromKubeconfigGetter is a helper function that builds configs from a master
// url and a kubeconfigGetter.
func BuildConfigFromKubeconfigGetter(masterUrl string, kubeconfigGetter KubeconfigGetter) (*restclient.Config, error) {
	// TODO: We do not need a DeferredLoader here. Refactor code and see if we can use DirectClientConfig here.
	cc := NewNonInteractiveDeferredLoadingClientConfig(
		&ClientConfigGetter{kubeconfigGetter: kubeconfigGetter},
		&ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: masterUrl}})
	return cc.ClientConfig()
}
