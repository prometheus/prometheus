// Copyright 2019 The Prometheus Authors
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
	"io/ioutil"
	"os"
	"strconv"
)

// A checkpoint file for remote write to use on restart/config reload
// in attempt to minimize the chance of missing data in the remote system.
type rwCheckpoint struct {
	f *os.File
}

// NewRemoteWriteCheckpoint creates a new file for the
// queue's remote write checkpoint if it doesn't already
// have one, otherwise it returns the timestamp that was
// contained in the existing file.
func NewRemoteWriteCheckpoint(name string) (rwCheckpoint, int64, error) {
	var c rwCheckpoint

	_, err := os.Stat(name)
	if err == nil {
		f, err := os.OpenFile(name, os.O_WRONLY, 0666)
		if err != nil {
			return rwCheckpoint{}, 0, err
		}
		c.f = f
		ts, err := c.readCheckpoint()
		return c, ts, err
	}
	f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return rwCheckpoint{}, 0, err
	}
	c.f = f
	return c, 0, nil
}

func (c *rwCheckpoint) writeCheckpoint(ts int64) error {
	err := c.f.Truncate(0)
	if err != nil {
		return err
	}

	_, err = c.f.WriteAt([]byte(strconv.FormatInt(ts, 10)), 0)
	if err != nil {
		return err
	}
	return nil
}

func (c *rwCheckpoint) readCheckpoint() (int64, error) {
	b, err := ioutil.ReadFile(c.f.Name())
	if err != nil {
		return 0, err
	}

	ts, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return 0, err
	}
	return ts, nil
}

func (c *rwCheckpoint) close() error {
	return c.f.Close()
}
