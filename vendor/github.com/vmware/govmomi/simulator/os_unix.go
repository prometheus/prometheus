//+build !windows

/*
Copyright (c) 2017 VMware, Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package simulator

import "syscall"

func (ds *Datastore) stat() error {
	info := ds.Info.GetDatastoreInfo()
	var stat syscall.Statfs_t

	err := syscall.Statfs(info.Url, &stat)
	if err != nil {
		return err
	}

	bsize := uint64(stat.Bsize) / 512

	info.FreeSpace = int64(stat.Bfree*bsize) >> 1

	ds.Summary.FreeSpace = info.FreeSpace
	ds.Summary.Capacity = int64(stat.Blocks*bsize) >> 1

	return nil
}
