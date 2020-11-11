package xmlrpc

import (
	"testing"
)

const faultRespXml = `
<?xml version="1.0" encoding="UTF-8"?>
<methodResponse>
  <fault>
    <value>
      <struct>
        <member>
          <name>faultString</name>
          <value>
            <string>You must log in before using this part of Bugzilla.</string>
          </value>
        </member>
        <member>
          <name>faultCode</name>
          <value>
            <int>410</int>
          </value>
        </member>
      </struct>
    </value>
  </fault>
</methodResponse>`

func Test_failedResponse(t *testing.T) {
	resp := Response([]byte(faultRespXml))

	if resp.Err() == nil {
		t.Fatal("Err() error: expected error, got nil")
	}

	fault := resp.Err().(FaultError)
	if fault.Code != 410 && fault.String != "You must log in before using this part of Bugzilla." {
		t.Fatal("Err() error: got wrong error")
	}
}

const emptyValResp = `
<?xml version="1.0" encoding="UTF-8"?>
<methodResponse>
	<params>
		<param>
			<value>
				<struct>
					<member>
						<name>user</name>
						<value><string>Joe Smith</string></value>
					</member>
					<member>
						<name>token</name>
						<value/>
					</member>
				</struct>
			</value>
		</param>
	</params>
</methodResponse>`

func Test_responseWithEmptyValue(t *testing.T) {
	resp := Response([]byte(emptyValResp))

	result := struct {
		User  string `xmlrpc:"user"`
		Token string `xmlrpc:"token"`
	}{}

	if err := resp.Unmarshal(&result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if result.User != "Joe Smith" || result.Token != "" {
		t.Fatalf("unexpected result: %v", result)
	}
}
