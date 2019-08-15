package errors

import (
	"strings"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/utils"
)

const SignatureDostNotMatchErrorCode = "SignatureDoesNotMatch"
const IncompleteSignatureErrorCode = "IncompleteSignature"
const MessageContain = "server string to sign is:"

var debug utils.Debug

func init() {
	debug = utils.Init("sdk")
}

type SignatureDostNotMatchWrapper struct {
}

func (*SignatureDostNotMatchWrapper) tryWrap(error *ServerError, wrapInfo map[string]string) (ok bool) {
	clientStringToSign := wrapInfo["StringToSign"]
	if (error.errorCode == SignatureDostNotMatchErrorCode || error.errorCode == IncompleteSignatureErrorCode) && clientStringToSign != "" {
		message := error.message
		if strings.Contains(message, MessageContain) {
			str := strings.Split(message, MessageContain)
			serverStringToSign := str[1]

			if clientStringToSign == serverStringToSign {
				// user secret is error
				error.recommend = "InvalidAccessKeySecret: Please check you AccessKeySecret"
			} else {
				debug("Client StringToSign: %s", clientStringToSign)
				debug("Server StringToSign: %s", serverStringToSign)
				error.recommend = "This may be a bug with the SDK and we hope you can submit this question in the " +
					"github issue(https://github.com/aliyun/alibaba-cloud-sdk-go/issues), thanks very much"
			}
		}
		ok = true
		return
	}
	ok = false
	return
}
