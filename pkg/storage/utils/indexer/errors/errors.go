// Copyright 2018-2022 CERN
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

package errors

import (
	"fmt"

	"github.com/cs3org/reva/pkg/storage/utils/indexer/option"
)

// AlreadyExistsErr implements the Error interface.
type AlreadyExistsErr struct {
	TypeName, Value string
	IndexBy         option.IndexBy
}

func (e *AlreadyExistsErr) Error() string {
	return fmt.Sprintf("%s with %s=%s does already exist", e.TypeName, e.IndexBy.String(), e.Value)
}

// IsAlreadyExistsErr checks whether an error is of type AlreadyExistsErr.
func IsAlreadyExistsErr(e error) bool {
	_, ok := e.(*AlreadyExistsErr)
	return ok
}

// NotFoundErr implements the Error interface.
type NotFoundErr struct {
	TypeName, Value string
	IndexBy         option.IndexBy
}

func (e *NotFoundErr) Error() string {
	return fmt.Sprintf("%s with %s=%s not found", e.TypeName, e.IndexBy.String(), e.Value)
}

// IsNotFoundErr checks whether an error is of type IsNotFoundErr.
func IsNotFoundErr(e error) bool {
	_, ok := e.(*NotFoundErr)
	return ok
}
