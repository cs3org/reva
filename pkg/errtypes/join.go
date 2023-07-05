// Copyright 2018-2023 CERN
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

package errtypes

import (
	"strings"
)

type joinErrors []error

// Join returns an error representing a list of errors.
func Join(err ...error) error {
	return joinErrors(err)
}

// Error return a string comma (,) separated of all the errors.
func (e joinErrors) Error() string {
	var b strings.Builder
	for i, err := range e {
		b.WriteString(err.Error())
		if i != len(e)-1 {
			b.WriteString(", ")
		}
	}
	return b.String()
}
