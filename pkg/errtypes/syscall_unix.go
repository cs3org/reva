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

// +build !windows

package errtypes

import (
	"github.com/pkg/xattr"
	"golang.org/x/sys/unix"
)

// XattrIsNoData checks if underlying error is ENODATA.
func XattrIsNoData(err error) bool {
	if xerr, ok := err.(*xattr.Error); ok {
		if serr, ok2 := xerr.Err.(unix.Errno); ok2 {
			return serr == ENODATA
		}
	}
	return false
}

// XattrIsNotFound checks if underlying error is ENOENT.
// The os not exists error is buried inside the xattr error,
// so we cannot just use os.IsNotExists().
func XattrIsNotFound(err error) bool {
	if xerr, ok := err.(*xattr.Error); ok {
		if serr, ok2 := xerr.Err.(unix.Errno); ok2 {
			return serr == unix.ENOENT
		}
	}
	return false
}
