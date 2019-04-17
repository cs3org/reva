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

// package err provides context to errors
// by wrapping them with contextual information.
// The current implementation relies on Dave Cheney's
// errors package. Go2 errors proposal
// will provide contextual errors.
// See https://go.googlesource.com/proposal/+/master/design/go2draft.md
package err

import (
	"fmt"
	"github.com/pkg/errors"
)

// type Error represents an error
// with contextual information.
type Err string

// New creates a new error that adds
// prefix to the contextual output.
func New(prefix string) Err {
	return Err(prefix)
}

func (e Err) build(message string) string {
	return string(e) + ": " + message
}

// Wrap wraps an error with contextual information and
// with the provided message.
func (e Err) Wrap(err error, message string) error {
	message = e.build(message)
	return errors.Wrap(err, message)
}

// Wrap wraps the error like Wrap but
// allows the use of formatted messages like fmt.Printf
// and derivates.
func (e Err) Wrapf(err error, format string, args ...interface{}) error {
	format = e.build(format)
	return errors.Wrapf(err, format, args...)
}

// Cause returns the root error after unwrapping
// all contextual layers.
func (e Err) Cause(err error) error {
	return errors.Cause(err)
}

// New creates a new error with the provided message.
func (e Err) New(message string) error {
	message = e.build(message)
	return errors.New(message)
}

// Newf is like New but allows formatted messages.
func (e Err) Newf(format string, args ...interface{}) error {
	message := e.build(format)
	message = fmt.Sprintf(format, args...)
	return errors.New(message)
}
