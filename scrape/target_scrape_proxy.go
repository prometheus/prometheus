// Copyright The Prometheus Authors
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

package scrape

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/prometheus/config"
)

var errTargetNotActive = errors.New("target is not active")

// ScrapeActiveTarget performs a one-off HTTP scrape of an active target in scrapePool using the pool's scrape client and configuration.
func (m *Manager) ScrapeActiveTarget(ctx context.Context, scrapePool, scrapeURL string, maxBytes int64, timeoutCap time.Duration) (contentType string, body []byte, _ error) {
	if scrapePool == "" {
		return "", nil, errors.New("scrape pool is required")
	}
	if scrapeURL == "" {
		return "", nil, errors.New("scrape URL is required")
	}
	if maxBytes <= 0 {
		return "", nil, errors.New("max bytes must be positive")
	}
	if timeoutCap <= 0 {
		return "", nil, errors.New("timeout cap must be positive")
	}

	m.mtxScrape.Lock()
	sp := m.scrapePools[scrapePool]
	m.mtxScrape.Unlock()
	if sp == nil {
		return "", nil, fmt.Errorf("scrape pool %q not found", scrapePool)
	}

	sp.targetMtx.Lock()
	var t *Target
	for _, at := range sp.activeTargets {
		if at.URL().String() == scrapeURL {
			t = at
			break
		}
	}
	sp.targetMtx.Unlock()
	if t == nil {
		return "", nil, errTargetNotActive
	}

	_, targetTimeout, err := t.intervalAndTimeout(time.Duration(sp.config.ScrapeInterval), time.Duration(sp.config.ScrapeTimeout))
	if err != nil {
		return "", nil, err
	}

	timeout := minDuration(timeoutCap, targetTimeout)

	bodyLimit := maxBytes
	if sp.config.BodySizeLimit > 0 && int64(sp.config.BodySizeLimit) < bodyLimit {
		bodyLimit = int64(sp.config.BodySizeLimit)
	}

	escapingScheme, _ := config.ToEscapingScheme(sp.config.MetricNameEscapingScheme, sp.config.MetricNameValidationScheme)
	s := &targetScraper{
		Target:               t,
		client:               sp.client,
		timeout:              timeout,
		bodySizeLimit:        bodyLimit,
		acceptHeader:         acceptHeader(sp.config.ScrapeProtocols, escapingScheme),
		acceptEncodingHeader: acceptEncodingHeader(sp.config.EnableCompression),
		metrics:              sp.metrics,
	}

	scrapeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := s.scrape(scrapeCtx)
	if err != nil {
		return "", nil, err
	}

	var buf bytes.Buffer
	ct, err := s.readResponse(scrapeCtx, resp, &buf)
	if err != nil {
		return "", nil, err
	}

	if len(ct) > 256 {
		ct = ct[:256]
	}
	return ct, buf.Bytes(), nil
}

func minDuration(a, b time.Duration) time.Duration {
	if a <= b {
		return a
	}
	return b
}
