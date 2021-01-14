// Copyright 2020 The Prometheus Authors
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

package web

import (
	"net/http"

	"github.com/go-kit/kit/log"
	"golang.org/x/crypto/bcrypt"
)

func validateUsers(configPath string) error {
	c, err := getConfig(configPath)
	if err != nil {
		return err
	}

	for _, p := range c.Users {
		_, err = bcrypt.Cost([]byte(p))
		if err != nil {
			return err
		}
	}

	return nil
}

type userAuthRoundtrip struct {
	tlsConfigPath string
	handler       http.Handler
	logger        log.Logger
}

func (u *userAuthRoundtrip) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := getConfig(u.tlsConfigPath)
	if err != nil {
		u.logger.Log("msg", "Unable to parse configuration", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	if len(c.Users) == 0 {
		u.handler.ServeHTTP(w, r)
		return
	}

	user, pass, auth := r.BasicAuth()
	if auth {
		if hashedPassword, ok := c.Users[user]; ok {
			if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(pass)); err == nil {
				u.handler.ServeHTTP(w, r)
				return
			}
		}
	}

	w.Header().Set("WWW-Authenticate", "Basic")
	http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
}
