// Copyright 2018-2024 CERN
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

package runtime

import (
	"context"

	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/rs/zerolog"
)

// Option defines a single option function.
type Option func(o *Options)

// Options defines the available options for this package.
type Options struct {
	Logger   *zerolog.Logger
	Registry registry.Registry
	PidFile  string
	Ctx      context.Context
}

// newOptions initializes the available default options.
func newOptions(opts ...Option) Options {
	l := zerolog.Nop()
	opt := Options{
		Logger: &l,
		Ctx:    context.TODO(),
	}

	for _, o := range opts {
		o(&opt)
	}

	return opt
}

// WithLogger provides a function to set the logger option.
func WithLogger(logger *zerolog.Logger) Option {
	return func(o *Options) {
		o.Logger = logger
	}
}

// WithPidFile sets to pidfile to use.
func WithPidFile(pidfile string) Option {
	return func(o *Options) {
		o.PidFile = pidfile
	}
}

// WithRegistry provides a function to set the registry.
func WithRegistry(r registry.Registry) Option {
	return func(o *Options) {
		o.Registry = r
	}
}

// WithContext sets the context to use.
func WithContext(ctx context.Context) Option {
	return func(o *Options) {
		o.Ctx = ctx
	}
}
