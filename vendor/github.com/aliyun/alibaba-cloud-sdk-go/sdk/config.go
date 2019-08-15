/*
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package sdk

import (
	"net/http"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/utils"
)

type Config struct {
	AutoRetry         bool            `default:"true"`
	MaxRetryTime      int             `default:"3"`
	UserAgent         string          `default:""`
	Debug             bool            `default:"false"`
	HttpTransport     *http.Transport `default:""`
	EnableAsync       bool            `default:"false"`
	MaxTaskQueueSize  int             `default:"1000"`
	GoRoutinePoolSize int             `default:"5"`
	Scheme            string          `default:"HTTP"`
	Timeout           time.Duration
}

func NewConfig() (config *Config) {
	config = &Config{}
	utils.InitStructWithDefaultTag(config)
	return
}

func (c *Config) WithAutoRetry(isAutoRetry bool) *Config {
	c.AutoRetry = isAutoRetry
	return c
}

func (c *Config) WithMaxRetryTime(maxRetryTime int) *Config {
	c.MaxRetryTime = maxRetryTime
	return c
}

func (c *Config) WithUserAgent(userAgent string) *Config {
	c.UserAgent = userAgent
	return c
}

func (c *Config) WithDebug(isDebug bool) *Config {
	c.Debug = isDebug
	return c
}

func (c *Config) WithTimeout(timeout time.Duration) *Config {
	c.Timeout = timeout
	return c
}

func (c *Config) WithHttpTransport(httpTransport *http.Transport) *Config {
	c.HttpTransport = httpTransport
	return c
}

func (c *Config) WithEnableAsync(isEnableAsync bool) *Config {
	c.EnableAsync = isEnableAsync
	return c
}

func (c *Config) WithMaxTaskQueueSize(maxTaskQueueSize int) *Config {
	c.MaxTaskQueueSize = maxTaskQueueSize
	return c
}

func (c *Config) WithGoRoutinePoolSize(goRoutinePoolSize int) *Config {
	c.GoRoutinePoolSize = goRoutinePoolSize
	return c
}

func (c *Config) WithScheme(scheme string) *Config {
	c.Scheme = scheme
	return c
}
