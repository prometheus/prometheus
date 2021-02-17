// Copyright 2021 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package remote

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	signer "github.com/aws/aws-sdk-go/aws/signer/v4"
	"github.com/prometheus/prometheus/config"
)

type sigV4RoundTripper struct {
	region string
	next   http.RoundTripper
	pool   sync.Pool

	signer *signer.Signer
}

// NewSigV4RoundTripper returns a new http.RoundTripper that will sign requests
// using Amazon's Signature Verification V4 signing procedure. The request will
// then be handed off to the next RoundTripper provided by next. If next is nil,
// http.DefaultTransport will be used.
//
// Credentials for signing are retrieved using the the default AWS credential
// chain. If credentials cannot be found, an error will be returned.
func NewSigV4RoundTripper(cfg config.SigV4Config, next http.RoundTripper) (http.RoundTripper, error) {
	if next == nil {
		next = http.DefaultTransport
	}

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(cfg.Region),
	})
	if err != nil {
		return nil, fmt.Errorf("could not create new AWS session: %w", err)
	}
	if _, err := sess.Config.Credentials.Get(); err != nil {
		return nil, fmt.Errorf("could not get SigV4 credentials: %w", err)
	}
	if aws.StringValue(sess.Config.Region) == "" {
		return nil, fmt.Errorf("region not configured in sigv4 or in default credentials chain")
	}

	rt := &sigV4RoundTripper{
		region: cfg.Region,
		next:   next,
		signer: signer.NewSigner(sess.Config.Credentials),
	}
	rt.pool.New = rt.newBuf
	return rt, nil
}

func (rt *sigV4RoundTripper) newBuf() interface{} {
	return bytes.NewBuffer(make([]byte, 0, 1024))
}

func (rt *sigV4RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// rt.signer.Sign needs a seekable body, so we replace the body with a
	// buffered reader filled with the contents of original body.
	buf := rt.pool.Get().(*bytes.Buffer)
	defer func() {
		buf.Reset()
		rt.pool.Put(buf)
	}()
	if _, err := io.Copy(buf, req.Body); err != nil {
		return nil, err
	}
	// Close the original body since we don't need it anymore.
	_ = req.Body.Close()

	// Ensure our seeker is back at the start of the buffer once we return.
	var seeker io.ReadSeeker = bytes.NewReader(buf.Bytes())
	defer func() {
		_, _ = seeker.Seek(0, io.SeekStart)
	}()
	req.Body = ioutil.NopCloser(seeker)

	_, err := rt.signer.Sign(req, seeker, "aps", rt.region, time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}
	return rt.next.RoundTrip(req)
}
