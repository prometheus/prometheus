// +build go1.8

/*
 *
 * Copyright 2017 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package credentials

import (
	"crypto/tls"
)

func init() {
	cipherSuiteLookup[tls.TLS_RSA_WITH_AES_128_CBC_SHA256] = "TLS_RSA_WITH_AES_128_CBC_SHA256"
	cipherSuiteLookup[tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256] = "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256"
	cipherSuiteLookup[tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256] = "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256"
	cipherSuiteLookup[tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305] = "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305"
	cipherSuiteLookup[tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305] = "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305"
}

// cloneTLSConfig returns a shallow clone of the exported
// fields of cfg, ignoring the unexported sync.Once, which
// contains a mutex and must not be copied.
//
// If cfg is nil, a new zero tls.Config is returned.
func cloneTLSConfig(cfg *tls.Config) *tls.Config {
	if cfg == nil {
		return &tls.Config{}
	}

	return cfg.Clone()
}
