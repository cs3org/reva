// Copyright 2018-2026 CERN
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

// Package http is the backend of the CLI over the public HTTP services of
// reva: WebDAV and TUS (ocdav), Graph (ocgraph), OCS and the archiver. It
// converts their answers to the CS3 messages. Some CS3 fields have no
// source over HTTPS, and stay empty: see the functions that convert.
package http

import (
	"context"
	"errors"
	"fmt"
	nethttp "net/http"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/internal/cli/cberr"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/cs3org/reva/v3/pkg/cli/backend/http/internal/client"
)

// Credential is a header that every request carries.
type Credential = client.Credential

// CredentialSource gives the credential of each request. A source that
// also has Renew(ctx, refused) gets one call after a 401, and the request
// is sent again once.
type CredentialSource = client.CredentialSource

// Options configure the backend.
type Options struct {
	HTTPClient  *nethttp.Client
	Credentials CredentialSource
	UserAgent   string
	// MaxRetries is the number of retries of a request that failed for a
	// transient reason. Negative means the default.
	MaxRetries int
}

// Backend implements backend.Backend and backend.Searcher.
type Backend struct {
	c *client.Client
}

var (
	_ backend.Backend  = (*Backend)(nil)
	_ backend.Searcher = (*Backend)(nil)
)

// New returns the backend of the HTTPS endpoint, https://host.
func New(endpoint string, o Options) (*Backend, error) {
	opts := []client.Option{client.WithCredentials(o.Credentials)}
	if o.HTTPClient != nil {
		opts = append(opts, client.WithHTTPClient(o.HTTPClient))
	}
	if o.UserAgent != "" {
		opts = append(opts, client.WithUserAgent(o.UserAgent))
	}
	if o.MaxRetries >= 0 {
		opts = append(opts, client.WithMaxRetries(o.MaxRetries))
	}
	c, err := client.New(endpoint, opts...)
	if err != nil {
		return nil, err
	}
	return &Backend{c: c}, nil
}

// Endpoint returns the base URL of the backend.
func (b *Backend) Endpoint() string { return b.c.Endpoint() }

func (b *Backend) Transport() string          { return backend.HTTP }
func (b *Backend) Files() backend.Files       { return files{b} }
func (b *Backend) Shares() backend.Shares     { return shares{b} }
func (b *Backend) Links() backend.Links       { return links{b} }
func (b *Backend) Spaces() backend.Spaces     { return spaceArea{b} }
func (b *Backend) Trash() backend.Trash       { return trash{b} }
func (b *Backend) Versions() backend.Versions { return versions{b} }
func (b *Backend) Locks() backend.Locks       { return locks{} }
func (b *Backend) OCM() backend.OCM           { return ocmArea{b} }
func (b *Backend) Tokens() backend.Tokens     { return tokens{b} }
func (b *Backend) Users() backend.Users       { return users{b} }
func (b *Backend) Locator() backend.Locator   { return locator{b} }

func (b *Backend) ServerTime(ctx context.Context) (time.Time, error) {
	t, err := b.c.ServerTime(ctx)
	return t, conv(err)
}

func notAvailable(what string) error { return backend.NotAvailable(backend.HTTP, what) }

// statusError is an error of the HTTPS API. Its text is the sentence of the
// client, and it also matches the *backend.StatusError of its CS3 code, so
// that commands can test the code on both transports.
type statusError struct {
	status *backend.StatusError
	cause  error
}

func (e *statusError) Error() string   { return e.cause.Error() }
func (e *statusError) Unwrap() []error { return []error{e.status, e.cause} }

// conv gives an error of the client its CS3 code. Errors without an HTTP
// status, such as a network error, stay as they are.
func conv(err error) error {
	ce, ok := errors.AsType[*cberr.Error](err)
	if !ok {
		return err
	}
	code := codeOf(ce)
	if code == rpc.Code_CODE_INVALID {
		return err
	}
	return &statusError{status: &backend.StatusError{Code: code, Msg: ce.Error()}, cause: err}
}

func codeOf(e *cberr.Error) rpc.Code {
	switch e.Status {
	case nethttp.StatusPreconditionFailed:
		return rpc.Code_CODE_FAILED_PRECONDITION
	case nethttp.StatusLocked:
		return rpc.Code_CODE_LOCKED
	case nethttp.StatusInsufficientStorage:
		return rpc.Code_CODE_INSUFFICIENT_STORAGE
	case nethttp.StatusNotImplemented:
		return rpc.Code_CODE_UNIMPLEMENTED
	case nethttp.StatusServiceUnavailable:
		return rpc.Code_CODE_UNAVAILABLE
	case nethttp.StatusTooManyRequests:
		return rpc.Code_CODE_RESOURCE_EXHAUSTED
	}
	switch e.Kind {
	case cberr.KindNotFound:
		return rpc.Code_CODE_NOT_FOUND
	case cberr.KindPermission:
		return rpc.Code_CODE_PERMISSION_DENIED
	case cberr.KindAuth:
		return rpc.Code_CODE_UNAUTHENTICATED
	case cberr.KindUsage:
		return rpc.Code_CODE_INVALID_ARGUMENT
	case cberr.KindConflict:
		if e.Status == 0 {
			return rpc.Code_CODE_ALREADY_EXISTS
		}
		return rpc.Code_CODE_ABORTED
	}
	if e.Status >= 500 {
		return rpc.Code_CODE_INTERNAL
	}
	if e.Status >= 400 {
		return rpc.Code_CODE_INVALID_ARGUMENT
	}
	return rpc.Code_CODE_INVALID
}

func notFound(format string, args ...any) error {
	return &backend.StatusError{Code: rpc.Code_CODE_NOT_FOUND, Msg: fmt.Sprintf(format, args...)}
}

func invalid(format string, args ...any) error {
	return &backend.StatusError{Code: rpc.Code_CODE_INVALID_ARGUMENT, Msg: fmt.Sprintf(format, args...)}
}
