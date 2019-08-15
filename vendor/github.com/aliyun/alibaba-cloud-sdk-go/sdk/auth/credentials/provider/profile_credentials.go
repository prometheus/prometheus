package provider

import (
	"bufio"
	"errors"
	"os"
	"runtime"
	"strings"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"

	ini "gopkg.in/ini.v1"
)

type ProfileProvider struct {
	Profile string
}

var ProviderProfile = NewProfileProvider()

// NewProfileProvider receive zero or more parameters,
// when length of name is 0, the value of field Profile will be "default",
// and when there are multiple inputs, the function will take the
// first one and  discard the other values.
func NewProfileProvider(name ...string) Provider {
	p := new(ProfileProvider)
	if len(name) == 0 {
		p.Profile = "default"
	} else {
		p.Profile = name[0]
	}
	return p
}

// Resolve implements the Provider interface
// when credential type is rsa_key_pair, the content of private_key file
// must be able to be parsed directly into the required string
// that NewRsaKeyPairCredential function needed
func (p *ProfileProvider) Resolve() (auth.Credential, error) {
	path, ok := os.LookupEnv(ENVCredentialFile)
	if !ok {
		path, err := checkDefaultPath()
		if err != nil {
			return nil, err
		}
		if path == "" {
			return nil, nil
		}
	} else if path == "" {
		return nil, errors.New("Environment variable '" + ENVCredentialFile + "' cannot be empty")
	}

	ini, err := ini.Load(path)
	if err != nil {
		return nil, errors.New("ERROR: Can not open file" + err.Error())
	}

	section, err := ini.GetSection(p.Profile)
	if err != nil {
		return nil, errors.New("ERROR: Can not load section" + err.Error())
	}

	value, err := section.GetKey("type")
	if err != nil {
		return nil, errors.New("ERROR: Can not find credential type" + err.Error())
	}

	switch value.String() {
	case "access_key":
		value1, err1 := section.GetKey("access_key_id")
		value2, err2 := section.GetKey("access_key_secret")
		if err1 != nil || err2 != nil {
			return nil, errors.New("ERROR: Failed to get value")
		}
		if value1.String() == "" || value2.String() == "" {
			return nil, errors.New("ERROR: Value can't be empty")
		}
		return credentials.NewAccessKeyCredential(value1.String(), value2.String()), nil
	case "ecs_ram_role":
		value1, err1 := section.GetKey("role_name")
		if err1 != nil {
			return nil, errors.New("ERROR: Failed to get value")
		}
		if value1.String() == "" {
			return nil, errors.New("ERROR: Value can't be empty")
		}
		return credentials.NewEcsRamRoleCredential(value1.String()), nil
	case "ram_role_arn":
		value1, err1 := section.GetKey("access_key_id")
		value2, err2 := section.GetKey("access_key_secret")
		value3, err3 := section.GetKey("role_arn")
		value4, err4 := section.GetKey("role_session_name")
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			return nil, errors.New("ERROR: Failed to get value")
		}
		if value1.String() == "" || value2.String() == "" || value3.String() == "" || value4.String() == "" {
			return nil, errors.New("ERROR: Value can't be empty")
		}
		return credentials.NewRamRoleArnCredential(value1.String(), value2.String(), value3.String(), value4.String(), 3600), nil
	case "rsa_key_pair":
		value1, err1 := section.GetKey("public_key_id")
		value2, err2 := section.GetKey("private_key_file")
		if err1 != nil || err2 != nil {
			return nil, errors.New("ERROR: Failed to get value")
		}
		if value1.String() == "" || value2.String() == "" {
			return nil, errors.New("ERROR: Value can't be empty")
		}
		file, err := os.Open(value2.String())
		if err != nil {
			return nil, errors.New("ERROR: Can not get private_key")
		}
		defer file.Close()
		var privateKey string
		scan := bufio.NewScanner(file)
		var data string
		for scan.Scan() {
			if strings.HasPrefix(scan.Text(), "----") {
				continue
			}
			data += scan.Text() + "\n"
		}
		return credentials.NewRsaKeyPairCredential(privateKey, value1.String(), 3600), nil
	default:
		return nil, errors.New("ERROR: Failed to get credential")
	}
}

// GetHomePath return home directory according to the system.
// if the environmental virables does not exist, will return empty
func GetHomePath() string {
	if runtime.GOOS == "windows" {
		path, ok := os.LookupEnv("USERPROFILE")
		if !ok {
			return ""
		}
		return path
	}
	path, ok := os.LookupEnv("HOME")
	if !ok {
		return ""
	}
	return path
}

func checkDefaultPath() (path string, err error) {
	path = GetHomePath()
	if path == "" {
		return "", errors.New("The default credential file path is invalid")
	}
	path = strings.Replace("~/.alibabacloud/credentials", "~", path, 1)
	_, err = os.Stat(path)
	if err != nil {
		return "", nil
	}
	return path, nil
}
