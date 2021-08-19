// Copyright 2018-2021 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

// +build freebsd

package localfs

import (
	"context"

	"golang.org/x/sys/unix"
)

func (fs *localfs) GetQuota(ctx context.Context) (uint64, uint64, error) {
	// TODO quota of which storage space?
	// we could use the logged in user, but when a user has access to multiple storages this falls short
	// for now return quota of root
	stat := unix.Statfs_t{}
	err := unix.Statfs(fs.wrap(ctx, "/"), &stat)
	if err != nil {
		return 0, 0, err
	}
	total := stat.Blocks * uint64(stat.Bsize)                        // Total data blocks in filesystem
	used := (stat.Blocks - uint64(stat.Bavail)) * uint64(stat.Bsize) // Free blocks available to unprivileged user
	return total, used, nil
}
