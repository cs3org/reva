// Copyright 2018-2021 CERN
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
	"time"

	"github.com/cs3org/reva/pkg/registry"
	"github.com/rs/zerolog"
)

// Option defines a single option function.
type Option func(o *Options)

// Options defines the available options for this package.
type Options struct {
	Logger              *zerolog.Logger
	Registry            registry.Registry
	ServiceName         string
	ServiceUUID         string
	NamespaceConfig     map[string]string
	RegistrationRefresh time.Duration
	Context             context.Context
}

// newOptions initializes the available default options.
func newOptions(opts ...Option) Options {
	opt := Options{
		Context: context.TODO(),
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

// WithRegistry provides a function to set the registry.
func WithRegistry(r registry.Registry) Option {
	return func(o *Options) {
		o.Registry = r
	}
}

// WithRegistrationRefresh provides a function to set the time for a registration refresh.
func WithRegistrationRefresh(t time.Duration) Option {
	return func(o *Options) {
		o.RegistrationRefresh = t
	}
}

// WithServiceName provides a function to set the service name.
func WithServiceName(n string) Option {
	return func(o *Options) {
		o.ServiceName = n
	}
}

// WithServiceUUID provides a function to set the service UUID.
func WithServiceUUID(uuid string) Option {
	return func(o *Options) {
		o.ServiceUUID = uuid
	}
}

func WithNameSpaceConfig(c map[string]string) Option {
	return func(o *Options) {
		o.NamespaceConfig = c
	}
}

func WithContext(c context.Context) Option {
	return func(o *Options) {
		o.Context = c
	}
}
