// Copyright (c) Bartłomiej Płotka @bwplotka
// Licensed under the Apache License 2.0.

package flagarize

import (
	"regexp"
)

type Regexp struct {
	*regexp.Regexp
}

// Set registers Regexp flag.
func (r *Regexp) Set(v string) (err error) {
	rg, err := regexp.Compile(v)
	if err != nil {
		return err
	}
	r.Regexp = rg
	return nil
}

type AnchoredRegexp struct {
	*regexp.Regexp
}

// Set registers anchored Regexp flag.
func (r *AnchoredRegexp) Set(v string) (err error) {
	rg, err := regexp.Compile("^(?:" + v + ")$")
	if err != nil {
		return err
	}
	r.Regexp = rg
	return nil
}
