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
	"time"

	"github.com/cenkalti/backoff"
	"github.com/cs3org/reva/v2/pkg/logger"
	"go-micro.dev/v4/events"

	"github.com/go-micro/plugins/v4/events/natsjs"
	nserver "github.com/nats-io/nats-server/v2/server"
)

// RunNatsServer starts the nats server and blocks
func RunNatsServer(opts ...Option) error {
	options := &nserver.Options{}

	for _, o := range opts {
		o(options)
	}

	server, err := nserver.NewServer(options)
	if err != nil {
		return err
	}

	c := &nserver.JetStreamConfig{}

	err = server.EnableJetStream(c)
	if err != nil {
		return err
	}

	server.Start()
	return nil
=======
	"fmt"

	"github.com/asim/go-micro/plugins/events/nats/v4"
	"github.com/cenkalti/backoff"
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
>>>>>>> master
}

// NewNatsStream returns a streaming client used by `Consume` and `Publish` methods
// retries exponentially to connect to a nats server
<<<<<<< HEAD
func NewNatsStream(opts ...natsjs.Option) (events.Stream, error) {
	b := backoff.NewExponentialBackOff()
	var stream events.Stream
	o := func() error {
		n := b.NextBackOff()
		s, err := natsjs.NewStream(opts...)
		if err != nil && n > time.Second {
			logger.New().Error().Err(err).Msgf("can't connect to nats (jetstream) server, retrying in %s", n)
=======
func NewNatsStream(opts ...nats.Option) (events.Stream, error) {
	b := backoff.NewExponentialBackOff()
	var stream events.Stream
	o := func() error {
		s, err := nats.NewStream(opts...)
		if err != nil {
			// TODO: should we get the standard logger here? if yes: How?
			fmt.Printf("can't connect to nats (stan) server, retrying in %s\n", b.NextBackOff())
>>>>>>> master
		}
		stream = s
		return err
	}

	err := backoff.Retry(o, b)
	return stream, err
}
