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

// Package utils contains utilities related to the notifications service and helper.
package utils

import (
	"time"

	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// ConnectToNats returns a resilient connection to the specified NATS server.
func ConnectToNats(natsAddress, natsToken string, log zerolog.Logger) (*nats.Conn, error) {
	nc, err := nats.Connect(
		natsAddress,
		nats.DrainTimeout(9*time.Second), // reva timeout on graceful shutdown is 10 seconds
		nats.MaxReconnects(-1),
		nats.Token(natsToken),
		nats.ErrorHandler(func(c *nats.Conn, s *nats.Subscription, err error) {
			log.Error().Err(err).Msgf("nats error")
		}),
		nats.ClosedHandler(func(c *nats.Conn) {
			if c.LastError() != nil {
				log.Error().Err(c.LastError()).Msgf("connection to nats server closed")
			} else {
				log.Debug().Msgf("connection to nats server closed")
			}
		}),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				log.Error().Err(err).Msgf("connection to nats server disconnected")
			}
		}),
		nats.CustomReconnectDelay(func(attempts int) time.Duration {
			if attempts%3 == 0 {
				log.Info().Msg("connection to nats server failed 3 times, backing off")
				return 5 * time.Minute
			}

			return 2 * time.Second
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			log.Info().Msgf("connection to nats server reconnected")
		}),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "connection to nats server at '%s' failed", natsAddress)
	}

	return nc, nil
}
