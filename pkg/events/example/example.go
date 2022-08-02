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

package main

import (
	"log"
	"time"

	"github.com/asim/go-micro/plugins/events/nats/v4"
	"github.com/cs3org/reva/pkg/events"
	"github.com/cs3org/reva/pkg/events/example/consumer"
	"github.com/cs3org/reva/pkg/events/example/publisher"
	"github.com/cs3org/reva/pkg/events/server"
)

// Simple example of an event workflow
func main() {
	// start a server
	Server()

	// obtain a client
	c := Client()

	// register a consumer
	go consumer.Example(c)

	// NOTE: consumer must be registered to get events
	time.Sleep(time.Millisecond)

	// Publish an event
	publisher.Example(c)

	// wait for consumer go-routine to print
	time.Sleep(500 * time.Millisecond)

}

// Server generates a nats server
func Server() {
	err := server.RunNatsServer()
	if err != nil {
		log.Fatal(err)
	}
}

// Client builds a nats client
func Client() events.Stream {
	c, err := server.NewNatsStream(nats.Address("127.0.0.1:4222"), nats.ClusterID("test-cluster"))
	if err != nil {
		log.Fatal(err)
	}
	return c

}
