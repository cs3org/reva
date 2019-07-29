// Copyright 2018-2019 CERN
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

package utils

import (
	typespb "github.com/cs3org/go-cs3apis/cs3/types"
)

// UnixNanoToTS converts a unix nano time to a valid cs3 Timestamp.
// TODO(labkode): review this code, optimize it?
func UnixNanoToTS(epoch uint64) *typespb.Timestamp {
	seconds := epoch / 1000000000
	nanos := epoch * 1000000000
	ts := &typespb.Timestamp{
		Nanos:   uint32(nanos),
		Seconds: seconds,
	}
	return ts
}
