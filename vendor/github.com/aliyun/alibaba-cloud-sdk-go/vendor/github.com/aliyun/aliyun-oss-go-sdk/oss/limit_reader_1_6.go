// +build !go1.7

// "golang.org/x/time/rate" is depended on golang context package  go1.7 onward
// this file is only for build,not supports limit upload speed
package oss

import (
	"fmt"
	"io"
)

const (
	perTokenBandwidthSize int = 1024
)

type OssLimiter struct {
}

type LimitSpeedReader struct {
	io.ReadCloser
	reader     io.Reader
	ossLimiter *OssLimiter
}

func GetOssLimiter(uploadSpeed int) (ossLimiter *OssLimiter, err error) {
	err = fmt.Errorf("rate.Limiter is not supported below version go1.7")
	return nil, err
}
