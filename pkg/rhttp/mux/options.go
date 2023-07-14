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

package mux

import (
	"strings"

	"github.com/cs3org/reva/pkg/rhttp/middlewares"
)

type Options struct {
	Unprotected bool
	Middlewares []middlewares.Middleware
}

type Option func(*Options)

func (o *Options) String() string {
	var b strings.Builder
	if o.Unprotected {
		b.WriteString("unprotected")
	}
	return b.String()
}

func (o *Options) apply(opts ...Option) {
	for _, opt := range opts {
		opt(o)
	}
}

func (o *Options) list() (opts []Option) {
	if o.Unprotected {
		opts = append(opts, Unprotected())
	}
	for _, m := range o.Middlewares {
		opts = append(opts, WithMiddleware(m))
	}
	return
}

func (o *Options) merge(other *Options) *Options {
	if o == nil {
		return other
	}
	opt := *o
	if other == nil {
		return &opt
	}
	opt.Unprotected = opt.Unprotected || other.Unprotected
	opt.Middlewares = append(other.Middlewares, opt.Middlewares...)
	return &opt
}

func Unprotected() Option {
	return func(o *Options) {
		o.Unprotected = true
	}
}

func WithMiddleware(middleware middlewares.Middleware) Option {
	return func(o *Options) {
		o.Middlewares = append(o.Middlewares, middleware)
	}
}
