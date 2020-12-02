// Copyright 2018-2020 CERN
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

// Package errtypes contains definitions for common errors.
// It would have nice to call this package errors, err or error
// but errors clashes with github.com/pkg/errors, err is used for any error variable
// and error is a reserved word :)
package errtypes

// NotFound is the error to use when a something is not found.
type NotFound string

func (e NotFound) Error() string { return "error: not found: " + string(e) }

// IsNotFound implements the IsNotFound interface.
func (e NotFound) IsNotFound() {}

// InternalError is the error to use when we really don't know what happened. Use with care
type InternalError string

func (e InternalError) Error() string { return "internal error: " + string(e) }

// IsInternalError implements the IsInternalError interface.
func (e InternalError) IsInternalError() {}

// PermissionDenied is the error to use when a resource cannot be access because of missing permissions.
type PermissionDenied string

func (e PermissionDenied) Error() string { return "error: permission denied: " + string(e) }

// IsPermissionDenied implements the IsPermissionDenied interface.
func (e PermissionDenied) IsPermissionDenied() {}

// AlreadyExists is the error to use when a resource something is not found.
type AlreadyExists string

func (e AlreadyExists) Error() string { return "error: already exists: " + string(e) }

// IsAlreadyExists implements the IsAlreadyExists interface.
func (e AlreadyExists) IsAlreadyExists() {}

// UserRequired represents an error when a resource is not found.
type UserRequired string

func (e UserRequired) Error() string { return "error: user required: " + string(e) }

// IsUserRequired implements the IsUserRequired interface.
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

// PartialContent is the error to use when the client request has partial data.
type PartialContent string

func (e PartialContent) Error() string { return "error: partial content: " + string(e) }

// IsPartialContent implements the IsPartialContent interface.
func (e PartialContent) IsPartialContent() {}

// BadRequest is the error to use when the server cannot or will not process the request (due to a client error). Reauthenticating won't help.
type BadRequest string

func (e BadRequest) Error() string { return "error: bad request: " + string(e) }

// IsBadRequest implements the IsBadRequest interface.
func (e BadRequest) IsBadRequest() {}

// IsNotFound is the interface to implement
// to specify that an a resource is not found.
type IsNotFound interface {
	IsNotFound()
}

// IsAlreadyExists is the interface to implement
// to specify that a resource already exists.
type IsAlreadyExists interface {
	IsAlreadyExists()
}

// IsInternalError is the interface to implement
// to specify that there was some internal error
type IsInternalError interface {
	IsInternalError()
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

// IsPermissionDenied is the interface to implement
// to specify that an action is denied.
type IsPermissionDenied interface {
	IsPermissionDenied()
}

// IsPartialContent is the interface to implement
// to specify that the client request has partial data.
type IsPartialContent interface {
	IsPartialContent()
}

// IsBadRequest is the interface to implement
// to specify that the server cannot or will not process the request.
type IsBadRequest interface {
	IsBadRequest()
}
