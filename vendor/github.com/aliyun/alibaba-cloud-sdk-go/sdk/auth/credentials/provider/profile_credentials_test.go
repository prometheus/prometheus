package provider_test

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"

	"github.com/stretchr/testify/assert"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials/provider"
)

var inistr = `
[default]              
enable = true                    
type = access_key                
access_key_id = foo               
access_key_secret = bar            
				   
[notype]              
access_key_id = foo               
access_key_secret = bar

[noak]
type = access_key                        
access_key_secret = bar   

[emptyak]
type = access_key                
access_key_id =                
access_key_secret = bar     

[ecs]                         
type = ecs_ram_role                
role_name = EcsRamRoleTest

[noecs]
type = ecs_ram_role                

[emptyecs]                         
type = ecs_ram_role                
role_name = 

[ram]                                         
type = ram_role_arn                
access_key_id = foo
access_key_secret = bar
role_arn = role_arn
role_session_name = session_name  

[noram]                                         
type = ram_role_arn                
access_key_secret = bar
role_arn = role_arn
role_session_name = session_name  

[emptyram]                                         
type = ram_role_arn                
access_key_id = 
access_key_secret = bar
role_arn = role_arn
role_session_name = session_name  

[rsa]                          
type = rsa_key_pair               
public_key_id = publicKeyId       
private_key_file = ./pk.pem

[norsa]                          
type = rsa_key_pair               
public_key_id = publicKeyId       

[emptyrsa]                          
type = rsa_key_pair               
public_key_id = publicKeyId       
private_key_file = 

[error_rsa]                          
type = rsa_key_pair               
public_key_id = publicKeyId       
private_key_file = ./pk_error.pem

[error_type]                          
type = error_type               
public_key_id = publicKeyId       
private_key_file = ./pk_error.pem
`
var privatekey = `this is privatekey`

func TestProfileProvider(t *testing.T) {
	var HOME string
	if runtime.GOOS == "windows" {
		HOME = "USERPROFILE"
	} else {
		HOME = "HOME"
	}
	path, ok := os.LookupEnv(HOME)
	assert.True(t, ok)
	os.Unsetenv(HOME)

	// testcase 1, no HOME or USERPROFILE environment variable set
	p := provider.NewProfileProvider()
	c, err := p.Resolve()
	assert.Nil(t, c)
	assert.EqualError(t, err, "The default credential file path is invalid")

	// testcase 2, default profile object
	os.Setenv(HOME, path)
	p = provider.NewProfileProvider()
	value, ok := p.(*provider.ProfileProvider)
	assert.True(t, ok)
	assert.Equal(t, value.Profile, "default")

	// testcase 3, credential file does not exist in the default path
	// and section name does not exist
	p = provider.NewProfileProvider("first")
	value, ok = p.(*provider.ProfileProvider)
	assert.True(t, ok)
	assert.Equal(t, value.Profile, "first")
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.Nil(t, err)

	// testcase 4, credential file path is error
	os.Setenv(provider.ENVCredentialFile, "../../credentials_error")
	p = provider.NewProfileProvider()
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.True(t, strings.Contains(err.Error(), "ERROR: Can not open file"))

	// create profile
	os.Setenv(provider.ENVCredentialFile, "./credentials")

	file, err := os.Create("./credentials")
	assert.Nil(t, err)
	file.WriteString(inistr)
	file.Close()
	defer os.Remove("./credentials")

	// testcase 5, section does not exist
	p = provider.NewProfileProvider("NonExist")
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.True(t, strings.Contains(err.Error(), "ERROR: Can not load section"))

	// testcase 6, credential type does not set
	p = provider.NewProfileProvider("notype")
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.True(t, strings.Contains(err.Error(), "ERROR: Can not find credential type"))

	// testcase 7, normal AK
	p = provider.NewProfileProvider()
	c, err = p.Resolve()
	assert.Equal(t, credentials.NewAccessKeyCredential("foo", "bar"), c)
	assert.Nil(t, err)
	// testcase 8, key does not exist
	p = provider.NewProfileProvider("noak")
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.True(t, strings.Contains(err.Error(), "ERROR: Failed to get value"))
	// testcase 9, value is empty
	p = provider.NewProfileProvider("emptyak")
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.True(t, strings.Contains(err.Error(), "ERROR: Value can't be empty"))

	//testcase 10, normal EcsRamRole
	p = provider.NewProfileProvider("ecs")
	c, err = p.Resolve()
	assert.Equal(t, credentials.NewEcsRamRoleCredential("EcsRamRoleTest"), c)
	assert.Nil(t, err)
	//testcase 11, key does not exist
	p = provider.NewProfileProvider("noecs")
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.True(t, strings.Contains(err.Error(), "ERROR: Failed to get value"))
	//testcase 12, value is empty
	p = provider.NewProfileProvider("emptyecs")
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.True(t, strings.Contains(err.Error(), "ERROR: Value can't be empty"))

	//testcase 13, normal RamRoleArn
	p = provider.NewProfileProvider("ram")
	c, err = p.Resolve()
	assert.Equal(t, credentials.NewRamRoleArnCredential("foo", "bar", "role_arn", "session_name", 3600), c)
	assert.Nil(t, err)
	//testcase 14, key does not exist
	p = provider.NewProfileProvider("noram")
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.True(t, strings.Contains(err.Error(), "ERROR: Failed to get value"))
	//testcase 15, value is empty
	p = provider.NewProfileProvider("emptyram")
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.True(t, strings.Contains(err.Error(), "ERROR: Value can't be empty"))

	//testase 16, normal RsaKeyPair
	file, err = os.Create("./pk.pem")
	assert.Nil(t, err)
	file.WriteString(privatekey)
	file.Close()

	p = provider.NewProfileProvider("rsa")
	c, err = p.Resolve()
	assert.Equal(t, credentials.NewRsaKeyPairCredential("", "publicKeyId", 3600), c)
	assert.Nil(t, err)
	defer os.Remove(`./pk.pem`)
	//testcase 17, key does not exist
	p = provider.NewProfileProvider("norsa")
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.True(t, strings.Contains(err.Error(), "ERROR: Failed to get value"))
	//testcase 18, value is empty
	p = provider.NewProfileProvider("emptyrsa")
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.True(t, strings.Contains(err.Error(), "ERROR: Value can't be empty"))

	//testcase 19, the value is error
	p = provider.NewProfileProvider("error_rsa")
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.True(t, strings.Contains(err.Error(), "ERROR: Can not get private_key"))

	//testcase 20, credential type is error
	p = provider.NewProfileProvider("error_type")
	c, err = p.Resolve()
	assert.Nil(t, c)
	assert.True(t, strings.Contains(err.Error(), "ERROR: Failed to get credential"))
}
