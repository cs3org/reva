// Package consumer contains an example implementation of an event consumer
package consumer

import (
	"fmt"
	"log"

	"github.com/cs3org/reva/pkg/events"
)

// Example consumes events from the queue
func Example(c events.Consumer) {
	// Step 1 - which group does the consumer belong to?
	// each group will get each event that is emitted, but only one member of the group will get it.
	group := "test-consumer"

	// Step 2 - which events does the consumer listen too?
	evs := []events.Unmarshaller{
		// for example created shares
		events.ShareCreated{},
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
		switch v := event.(type) {
		case events.ShareCreated:
			fmt.Printf("%s) Share created: %+v\n", group, v)
		default:
			fmt.Printf("%s) Unregistered event: %+v\n", group, v)
		}
	}

}
