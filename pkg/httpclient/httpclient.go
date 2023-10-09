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

package httpclient

import (
	"errors"
	"net/http"
	"time"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/trace"
)

// TODO(labkode): harden it.
// https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
func New(opts ...Option) *Client {
	options := newOptions(opts...)

	var tr http.RoundTripper
	if options.RoundTripper == nil {
		tr = &injectTransport{rt: http.DefaultTransport}
	} else {
		tr = &injectTransport{rt: options.RoundTripper}
	}

	httpClient := &http.Client{
		Timeout:   options.Timeout,
		Transport: tr,
	}

	return &Client{c: httpClient}
}

// Option defines a single option function.
type Option func(o *Options)

// Options defines the available options for this package.
type Options struct {
	Jar           http.CookieJar
	CheckRedirect func(req *http.Request, via []*http.Request) error
	Timeout       time.Duration
	RoundTripper  http.RoundTripper
}

// newOptions initializes the available default options.
func newOptions(opts ...Option) Options {
	opt := Options{}

	for _, o := range opts {
		o(&opt)
	}

	return opt
}

// Timeout provides a function to set the timeout option.
func Timeout(t time.Duration) Option {
	return func(o *Options) {
		o.Timeout = t
	}
}

// RoundTripper provides a function to set a custom RoundTripper.
func RoundTripper(rt http.RoundTripper) Option {
	return func(o *Options) {
		o.RoundTripper = rt
	}
}

// CheckRedirect provides a function to set a custom CheckRedirect.
func CheckRedirect(cr func(req *http.Request, via []*http.Request) error) Option {
	return func(o *Options) {
		o.CheckRedirect = cr
	}
}

// Jar provides a function to set a custom CookieJar.
func Jar(j http.CookieJar) Option {
	return func(o *Options) {
		o.Jar = j
	}
}

// Client wraps a http.Client but only exposes the Do method
// to force consumers to always create a request with http.NewRequestWithContext().
type Client struct {
	c *http.Client
}

func (c *Client) Do(r *http.Request) (*http.Response, error) {
	// bail out early if context is not set
	if r.Context() == nil {
		return nil, errors.New("error: request must have a context")
	}
	return c.c.Do(r)
}

func (c *Client) GetNativeHTTP() *http.Client {
	return c.c
}

type injectTransport struct {
	rt http.RoundTripper
}

func (t injectTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx := r.Context()

	traceID := trace.Get(ctx)

	r.Header.Set("X-Trace-ID", traceID)

	tkn, ok := appctx.ContextGetToken(ctx)
	if ok {
		r.Header.Set(appctx.TokenHeader, tkn)
	}

	return t.rt.RoundTrip(r)
}
