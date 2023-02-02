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

// Package consumer contains an example implementation of an event consumer
package consumer

import (
	"fmt"
	"log"

	"github.com/cs3org/reva/v2/pkg/events"
)

// Example consumes events from the queue
func Example(c events.Consumer) {
	// Step 1 - which group does the consumer belong to?
	// each group will get each event that is emitted, but only one member of the group will get it.
	group := "test-consumer"

	// Step 2 - which events does the consumer listen too?
	evs := []events.Unmarshaller{
		events.ShareCreated{},
		events.ShareUpdated{},
		events.ShareRemoved{},
		events.ReceivedShareUpdated{},
		events.LinkCreated{},
		events.LinkUpdated{},
		events.LinkRemoved{},
		events.LinkAccessed{},
		events.LinkAccessFailed{},
	}

	// Step 3 - create event channel
	evChan, err := events.Consume(c, group, evs...)
	if err != nil {
		log.Fatal(err)
	}

	// Step 4 - listen to events
	for {
		event := <-evChan

		// best to use type switch to differentiate events
		switch v := event.Event.(type) {
		case events.ShareCreated:
			fmt.Printf("%s) Share created: %+v\n", group, v)
		default:
			fmt.Printf("%s) %T: %+v\n", group, v, v)
		}
	}

}
