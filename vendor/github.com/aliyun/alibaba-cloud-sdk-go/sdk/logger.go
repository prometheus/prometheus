package sdk

import (
	"encoding/json"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/utils"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

var logChannel string
var defaultChannel = "AlibabaCloud"

type Logger struct {
	*log.Logger
	formatTemplate string
	isOpen         bool
	lastLogMsg     string
}

var defaultLoggerTemplate = `{time} {channel}: "{method} {uri} HTTP/{version}" {code} {cost} {hostname}`
var loggerParam = []string{"{time}", "{start_time}", "{ts}", "{channel}", "{pid}", "{host}", "{method}", "{uri}", "{version}", "{target}", "{hostname}", "{code}", "{error}", "{req_headers}", "{res_body}", "{res_headers}", "{cost}"}

func initLogMsg(fieldMap map[string]string) {
	for _, value := range loggerParam {
		fieldMap[value] = ""
	}
}

func (client *Client) GetLogger() *Logger {
	return client.logger
}

func (client *Client) GetLoggerMsg() string {
	if client.logger == nil {
		client.SetLogger("", "", os.Stdout, "")
	}
	return client.logger.lastLogMsg
}

func (client *Client) SetLogger(level string, channel string, out io.Writer, template string) {
	if level == "" {
		level = "info"
	}

	logChannel = "AlibabaCloud"
	if channel != "" {
		logChannel = channel
	}
	log := log.New(out, "["+strings.ToUpper(level)+"]", log.Lshortfile)
	if template == "" {
		template = defaultLoggerTemplate
	}

	client.logger = &Logger{
		Logger:         log,
		formatTemplate: template,
		isOpen:         true,
	}
}

func (client *Client) OpenLogger() {
	if client.logger == nil {
		client.SetLogger("", "", os.Stdout, "")
	}
	client.logger.isOpen = true
}

func (client *Client) CloseLogger() {
	if client.logger != nil {
		client.logger.isOpen = false
	}
}

func (client *Client) SetTemplate(template string) {
	if client.logger == nil {
		client.SetLogger("", "", os.Stdout, "")
	}
	client.logger.formatTemplate = template
}

func (client *Client) GetTemplate() string {
	if client.logger == nil {
		client.SetLogger("", "", os.Stdout, "")
	}
	return client.logger.formatTemplate
}

func TransToString(object interface{}) string {
	byt, err := json.Marshal(object)
	if err != nil {
		return ""
	}
	return string(byt)
}

func (client *Client) printLog(fieldMap map[string]string, err error) {
	if err != nil {
		fieldMap["{error}"] = err.Error()
	}
	fieldMap["{time}"] = time.Now().Format("2006-01-02 15:04:05")
	fieldMap["{ts}"] = utils.GetTimeInFormatISO8601()
	fieldMap["{channel}"] = logChannel
	if client.logger != nil {
		logMsg := client.logger.formatTemplate
		for key, value := range fieldMap {
			logMsg = strings.Replace(logMsg, key, value, -1)
		}
		client.logger.lastLogMsg = logMsg
		if client.logger.isOpen == true {
			client.logger.Output(2, logMsg)
		}
	}
}
