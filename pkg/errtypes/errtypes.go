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

// Package errtypes contains definitons for common errors.
// It would have nice to call this package errors, err or error
// but errors clashes with github.com/pkg/errors, err is used for any error variable
// and error is a reserved word :)
package errtypes

// NotFound is the error to use when a resource something is not found.
type NotFound string

func (e NotFound) Error() string { return "error: not found: " + string(e) }

// IsNotFound is the method to check for w
func (e NotFound) IsNotFound() {}

// AlreadyExists is the error to use when a resource something is not found.
type AlreadyExists string

func (e AlreadyExists) Error() string { return "error: already exists: " + string(e) }

// IsAlreadyExists is the method to check for w
func (e AlreadyExists) IsAlreadyExists() {}

// UserRequired represents an error when a resource is not found.
type UserRequired string

func (e UserRequired) Error() string { return "error: user required: " + string(e) }

// IsUserRequired implements the UserRequired interface.
func (e UserRequired) IsUserRequired() {}

// InvalidCredentials is the error to use when receiving invalid credentials.
type InvalidCredentials string

func (e InvalidCredentials) Error() string { return "error: invalid credentials: " + string(e) }

// IsInvalidCredentials implements the IsInvalidCredentials interface.
func (e InvalidCredentials) IsInvalidCredentials() {}

// NotSupported is the error to use when an action is not supported.
type NotSupported string

func (e NotSupported) Error() string { return "error: not supported: " + string(e) }

// IsNotSupported implements the IsNotSupported interface.
func (e NotSupported) IsNotSupported() {}

// IsNotFound is the interface to implement
// to specify that an a resource is not found.
type IsNotFound interface {
	IsNotFound()
}

// IsAlreadyExists is the interface to implement
// to specify that an a resource is not found.
type IsAlreadyExists interface {
	IsAlreadyExists()
}

// IsUserRequired is the interface to implement
// to specify that a user is required.
type IsUserRequired interface {
	IsUserRequired()
}

// IsInvalidCredentials is the interface to implement
// to specify that credentials were wrong.
type IsInvalidCredentials interface {
	IsInvalidCredentials()
}

// IsNotSupported is the interface to implement
// to specify that an action is not supported.
type IsNotSupported interface {
	IsNotSupported()
}
