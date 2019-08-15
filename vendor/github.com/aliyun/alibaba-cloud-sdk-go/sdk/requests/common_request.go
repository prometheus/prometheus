package requests

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
)

type CommonRequest struct {
	*baseRequest

	Version     string
	ApiName     string
	Product     string
	ServiceCode string

	// roa params
	PathPattern string
	PathParams  map[string]string

	Ontology AcsRequest
}

func NewCommonRequest() (request *CommonRequest) {
	request = &CommonRequest{
		baseRequest: defaultBaseRequest(),
	}
	request.Headers["x-sdk-invoke-type"] = "common"
	request.PathParams = make(map[string]string)
	return
}

func (request *CommonRequest) String() string {
	request.TransToAcsRequest()

	resultBuilder := bytes.Buffer{}

	mapOutput := func(m map[string]string) {
		if len(m) > 0 {
			sortedKeys := make([]string, 0)
			for k := range m {
				sortedKeys = append(sortedKeys, k)
			}

			// sort 'string' key in increasing order
			sort.Strings(sortedKeys)

			for _, key := range sortedKeys {
				resultBuilder.WriteString(key + ": " + m[key] + "\n")
			}
		}
	}

	// Request Line
	resultBuilder.WriteString(fmt.Sprintf("%s %s %s/1.1\n", request.Method, request.BuildQueries(), strings.ToUpper(request.Scheme)))

	// Headers
	resultBuilder.WriteString("Host" + ": " + request.Domain + "\n")
	mapOutput(request.Headers)

	resultBuilder.WriteString("\n")
	// Body
	if len(request.Content) > 0 {
		resultBuilder.WriteString(string(request.Content) + "\n")
	} else {
		mapOutput(request.FormParams)
	}

	return resultBuilder.String()
}

func (request *CommonRequest) TransToAcsRequest() {
	if len(request.PathPattern) > 0 {
		roaRequest := &RoaRequest{}
		roaRequest.initWithCommonRequest(request)
		request.Ontology = roaRequest
	} else {
		rpcRequest := &RpcRequest{}
		rpcRequest.baseRequest = request.baseRequest
		rpcRequest.product = request.Product
		rpcRequest.version = request.Version
		rpcRequest.locationServiceCode = request.ServiceCode
		rpcRequest.actionName = request.ApiName
		request.Ontology = rpcRequest
	}
}

func (request *CommonRequest) BuildUrl() string {
	return request.Ontology.BuildUrl()
}

func (request *CommonRequest) BuildQueries() string {
	return request.Ontology.BuildQueries()
}

func (request *CommonRequest) GetBodyReader() io.Reader {
	return request.Ontology.GetBodyReader()
}

func (request *CommonRequest) GetStyle() string {
	return request.Ontology.GetStyle()
}

func (request *CommonRequest) addPathParam(key, value string) {
	request.PathParams[key] = value
}
