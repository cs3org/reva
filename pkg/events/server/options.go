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

package server

import (
<<<<<<< HEAD
	nserver "github.com/nats-io/nats-server/v2/server"
)

// Option configures the nats server
type Option func(*nserver.Options)

// Host sets the host URL for the nats server
func Host(url string) Option {
	return func(o *nserver.Options) {
		o.Host = url
=======
	natsServer "github.com/nats-io/nats-server/v2/server"
	stanServer "github.com/nats-io/nats-streaming-server/server"
)

// Option configures the nats server
type Option func(*natsServer.Options, *stanServer.Options)

// Host sets the host URL for the nats server
func Host(url string) Option {
	return func(no *natsServer.Options, _ *stanServer.Options) {
		no.Host = url
>>>>>>> master
	}
}

// Port sets the host URL for the nats server
func Port(port int) Option {
<<<<<<< HEAD
	return func(o *nserver.Options) {
		o.Port = port
	}
}

// ClusterID sets the name for the nats cluster
func ClusterID(clusterID string) Option {
	return func(o *nserver.Options) {
		o.Cluster.Name = clusterID
=======
	return func(no *natsServer.Options, _ *stanServer.Options) {
		no.Port = port
	}
}

// NatsOpts allows setting Options from nats package directly
func NatsOpts(opt func(*natsServer.Options)) Option {
	return func(no *natsServer.Options, _ *stanServer.Options) {
		opt(no)
	}
}

// StanOpts allows setting Options from stan package directly
func StanOpts(opt func(*stanServer.Options)) Option {
	return func(_ *natsServer.Options, so *stanServer.Options) {
		opt(so)
>>>>>>> master
	}
}
