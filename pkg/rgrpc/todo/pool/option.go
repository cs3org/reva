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

package pool

const (
	defaultMaxCallRecvMsgSize = 10240000
)

// Option defines a single option function.
type Option func(o *Options)

// Options defines the available options for this package.
type Options struct {
	Endpoint           string
	MaxCallRecvMsgSize int
}

// newOptions initializes the available default options.
func newOptions(opts ...Option) Options {
	opt := Options{
		MaxCallRecvMsgSize: defaultMaxCallRecvMsgSize,
	}

	for _, o := range opts {
		o(&opt)
	}

	return opt
}

// Endpoint provides a function to set the endpoint option.
func Endpoint(val string) Option {
	return func(o *Options) {
		o.Endpoint = val
	}
}

// MaxCallRecvMsgSize provides a function to set the MaxCallRecvMsgSize option.
func MaxCallRecvMsgSize(size int) Option {
	return func(o *Options) {
		o.MaxCallRecvMsgSize = size
	}
}
