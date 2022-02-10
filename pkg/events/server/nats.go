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
	"github.com/asim/go-micro/plugins/events/nats/v4"
	"go-micro.dev/v4/events"

	stanServer "github.com/nats-io/nats-streaming-server/server"
)

// RunNatsServer runs the nats streaming server
func RunNatsServer(opts ...Option) error {
	natsOpts := stanServer.DefaultNatsServerOptions
	stanOpts := stanServer.GetDefaultOptions()

	for _, o := range opts {
		o(&natsOpts, stanOpts)
	}
	_, err := stanServer.RunServerWithOpts(stanOpts, &natsOpts)
	return err
}

// NewNatsStream returns a streaming client used by `Consume` and `Publish` methods
func NewNatsStream(opts ...nats.Option) (events.Stream, error) {
	return nats.NewStream(opts...)
}
