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

package sharesstorageprovider

import (
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	"github.com/cs3org/reva/v2/pkg/storage/cache"
)

// Option is used to pass options
type Option func(opts *Options)

// Options represent options
type Options struct {
	GatewayAPIClient       gateway.GatewayAPIClient
	CollaborationAPIClient collaboration.CollaborationAPIClient
	StatCache              cache.StatCache
}

// WithGatewayAPIClient allows to set the gateway selector option
func WithGatewayAPIClient(v gateway.GatewayAPIClient) Option {
	return func(o *Options) {
		o.GatewayAPIClient = v
	}
}

// WithCollaborationAPIClient allows to set the opentelemetry tracer provider for grpc clients
func WithCollaborationAPIClient(v collaboration.CollaborationAPIClient) Option {
	return func(o *Options) {
		o.CollaborationAPIClient = v
	}
}

// WithStatCache allows to set the registry for service lookup
func WithStatCache(v cache.StatCache) Option {
	return func(o *Options) {
		o.StatCache = v
	}
}
