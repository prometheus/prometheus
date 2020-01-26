package cli

// Copyright 2017 Microsoft Corporation
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"time"

	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/date"
	"github.com/mitchellh/go-homedir"
)

// Token represents an AccessToken from the Azure CLI
type Token struct {
	AccessToken      string `json:"accessToken"`
	Authority        string `json:"_authority"`
	ClientID         string `json:"_clientId"`
	ExpiresOn        string `json:"expiresOn"`
	IdentityProvider string `json:"identityProvider"`
	IsMRRT           bool   `json:"isMRRT"`
	RefreshToken     string `json:"refreshToken"`
	Resource         string `json:"resource"`
	TokenType        string `json:"tokenType"`
	UserID           string `json:"userId"`
}

// ToADALToken converts an Azure CLI `Token`` to an `adal.Token``
func (t Token) ToADALToken() (converted adal.Token, err error) {
	tokenExpirationDate, err := ParseExpirationDate(t.ExpiresOn)
	if err != nil {
		err = fmt.Errorf("Error parsing Token Expiration Date %q: %+v", t.ExpiresOn, err)
		return
	}

	difference := tokenExpirationDate.Sub(date.UnixEpoch())

	converted = adal.Token{
		AccessToken:  t.AccessToken,
		Type:         t.TokenType,
		ExpiresIn:    "3600",
		ExpiresOn:    json.Number(strconv.Itoa(int(difference.Seconds()))),
		RefreshToken: t.RefreshToken,
		Resource:     t.Resource,
	}
	return
}

// AccessTokensPath returns the path where access tokens are stored from the Azure CLI
// TODO(#199): add unit test.
func AccessTokensPath() (string, error) {
	// Azure-CLI allows user to customize the path of access tokens thorugh environment variable.
	var accessTokenPath = os.Getenv("AZURE_ACCESS_TOKEN_FILE")
	var err error

	// Fallback logic to default path on non-cloud-shell environment.
	// TODO(#200): remove the dependency on hard-coding path.
	if accessTokenPath == "" {
		accessTokenPath, err = homedir.Expand("~/.azure/accessTokens.json")
	}

	return accessTokenPath, err
}

// ParseExpirationDate parses either a Azure CLI or CloudShell date into a time object
func ParseExpirationDate(input string) (*time.Time, error) {
	// CloudShell (and potentially the Azure CLI in future)
	expirationDate, cloudShellErr := time.Parse(time.RFC3339, input)
	if cloudShellErr != nil {
		// Azure CLI (Python) e.g. 2017-08-31 19:48:57.998857 (plus the local timezone)
		const cliFormat = "2006-01-02 15:04:05.999999"
		expirationDate, cliErr := time.ParseInLocation(cliFormat, input, time.Local)
		if cliErr == nil {
			return &expirationDate, nil
		}

		return nil, fmt.Errorf("Error parsing expiration date %q.\n\nCloudShell Error: \n%+v\n\nCLI Error:\n%+v", input, cloudShellErr, cliErr)
	}

	return &expirationDate, nil
}

// LoadTokens restores a set of Token objects from a file located at 'path'.
func LoadTokens(path string) ([]Token, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file (%s) while loading token: %v", path, err)
	}
	defer file.Close()

	var tokens []Token

	dec := json.NewDecoder(file)
	if err = dec.Decode(&tokens); err != nil {
		return nil, fmt.Errorf("failed to decode contents of file (%s) into a `cli.Token` representation: %v", path, err)
	}

	return tokens, nil
}

// GetTokenFromCLI gets a token using Azure CLI 2.0 for local development scenarios.
func GetTokenFromCLI(resource string) (*Token, error) {
	// This is the path that a developer can set to tell this class what the install path for Azure CLI is.
	const azureCLIPath = "AzureCLIPath"

	// The default install paths are used to find Azure CLI. This is for security, so that any path in the calling program's Path environment is not used to execute Azure CLI.
	azureCLIDefaultPathWindows := fmt.Sprintf("%s\\Microsoft SDKs\\Azure\\CLI2\\wbin; %s\\Microsoft SDKs\\Azure\\CLI2\\wbin", os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"))

	// Default path for non-Windows.
	const azureCLIDefaultPath = "/usr/bin:/usr/local/bin"

	// Validate resource, since it gets sent as a command line argument to Azure CLI
	const invalidResourceErrorTemplate = "Resource %s is not in expected format. Only alphanumeric characters, [dot], [colon], [hyphen], and [forward slash] are allowed."
	match, err := regexp.MatchString("^[0-9a-zA-Z-.:/]+$", resource)
	if err != nil {
		return nil, err
	}
	if !match {
		return nil, fmt.Errorf(invalidResourceErrorTemplate, resource)
	}

	// Execute Azure CLI to get token
	var cliCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cliCmd = exec.Command(fmt.Sprintf("%s\\system32\\cmd.exe", os.Getenv("windir")))
		cliCmd.Env = os.Environ()
		cliCmd.Env = append(cliCmd.Env, fmt.Sprintf("PATH=%s;%s", os.Getenv(azureCLIPath), azureCLIDefaultPathWindows))
		cliCmd.Args = append(cliCmd.Args, "/c")
	} else {
		cliCmd = exec.Command(os.Getenv("SHELL"))
		cliCmd.Env = os.Environ()
		cliCmd.Env = append(cliCmd.Env, fmt.Sprintf("PATH=%s:%s", os.Getenv(azureCLIPath), azureCLIDefaultPath))
	}
	cliCmd.Args = append(cliCmd.Args, "az", "account", "get-access-token", "-o", "json", "--resource", resource)

	var stderr bytes.Buffer
	cliCmd.Stderr = &stderr

	output, err := cliCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Invoking Azure CLI failed with the following error: %s", stderr.String())
	}

	tokenResponse := Token{}
	err = json.Unmarshal(output, &tokenResponse)
	if err != nil {
		return nil, err
	}

	return &tokenResponse, err
}
