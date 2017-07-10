// Copyright 2012-2016 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

// This file is (c) 2014 Cenk Altı and governed by the MIT license.
// See https://github.com/cenkalti/backoff for original source.

package backoff

import (
	"errors"
	"log"
	"testing"
	"time"
)

func TestRetry(t *testing.T) {
	const successOn = 3
	var i = 0

	// This function is successfull on "successOn" calls.
	f := func() error {
		i++
		log.Printf("function is called %d. time\n", i)

		if i == successOn {
			log.Println("OK")
			return nil
		}

		log.Println("error")
		return errors.New("error")
	}

	min := time.Duration(8) * time.Millisecond
	max := time.Duration(256) * time.Millisecond
	err := Retry(f, NewExponentialBackoff(min, max).SendStop(true))
	if err != nil {
		t.Errorf("unexpected error: %s", err.Error())
	}
	if i != successOn {
		t.Errorf("invalid number of retries: %d", i)
	}
}
