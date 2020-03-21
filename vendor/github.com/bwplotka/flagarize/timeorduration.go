// Copyright (c) Bartłomiej Płotka @bwplotka
// Licensed under the Apache License 2.0.

// Taken from Thanos project.
//
// Copyright (c) The Thanos Authors.
// Licensed under the Apache License 2.0.

package flagarize

import (
	"bytes"
	"fmt"
	"time"

	"github.com/bwplotka/flagarize/internal/timestamp"
)

// multiError is a slice of errors implementing the error interface. It is used
// by a Gatherer to report multiple errors during MetricFamily gathering.
type multiError []error

func (errs multiError) Error() string {
	if len(errs) == 0 {
		return ""
	}
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "%d error(s) occurred:", len(errs))
	for _, err := range errs {
		fmt.Fprintf(buf, "\n* %s", err)
	}
	return buf.String()
}

// Append appends the provided error if it is not nil.
func (errs *multiError) Append(err error) {
	if err != nil {
		*errs = append(*errs, err)
	}
}

// TimeOrDuration is a custom kingping parser for time in RFC3339
// or duration in Go's duration format, such as "300ms", "-1.5h" or "2h45m".
// Only one will be set.
type TimeOrDuration struct {
	Time *time.Time
	Dur  *time.Duration
}

// Set converts string to TimeOrDuration.
func (tdv *TimeOrDuration) Set(s string) error {
	var merr multiError
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		tdv.Time = &t
		return nil
	}
	merr.Append(err)

	// error parsing time, let's try duration.
	var minus bool
	if s[0] == '-' {
		minus = true
		s = s[1:]
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		merr.Append(err)
		return merr
	}

	if minus {
		dur = dur * -1
	}
	tdv.Dur = &dur
	return nil
}

// String returns either time or duration.
func (tdv *TimeOrDuration) String() string {
	switch {
	case tdv.Time != nil:
		return tdv.Time.String()
	case tdv.Dur != nil:
		return tdv.Dur.String()
	}

	return "nil"
}

// PrometheusTimestamp returns TimeOrDuration converted to PrometheusTimestamp
// if duration is set now+duration is converted to Timestamp.
func (tdv *TimeOrDuration) PrometheusTimestamp() int64 {
	switch {
	case tdv.Time != nil:
		return timestamp.FromTime(*tdv.Time)
	case tdv.Dur != nil:
		return timestamp.FromTime(time.Now().Add(time.Duration(*tdv.Dur)))
	}

	return 0
}
