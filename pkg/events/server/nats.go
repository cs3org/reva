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
}
