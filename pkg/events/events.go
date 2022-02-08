package events

import (
	"log"
	"reflect"

	"go-micro.dev/v4/events"
)

var (
	// MainQueueName is the name of the main queue
	// All events will go through here as they are forwarded to the consumer via the
	// group name
	// TODO: "fan-out" so not all events go through the same queue? requires investigation
	MainQueueName = "main-queue"

	// MetadatakeyEventType is the key used for the eventtype in the metadata map of the event
	MetadatakeyEventType = "eventtype"
)

// Consume returns a channel that will get all events emitted by the system
// group defines the service type: One group will get exactly one copy of a event that is emitted
// NOTE: uses reflect on initialization
func Consume(group string, s events.Stream) (<-chan interface{}, error) {
	c, err := s.Consume(MainQueueName, events.WithGroup(group))
	if err != nil {
		return nil, err
	}

	outchan := make(chan interface{})
	go func() {
		for {
			e := <-c

			et := e.Metadata[MetadatakeyEventType]
			event, err := UnmarshalEvent(et, e.Payload)
			if err != nil {
				log.Printf("can't unmarshal event %v", err)
				continue
			}

			outchan <- event
		}
	}()
	return outchan, nil
}

// Publish publishes the ev to the MainQueue from where it is distributed to all subscribers
// NOTE: needs to use reflect on runtime
func Publish(ev interface{}, s events.Stream) error {
	evName := reflect.TypeOf(ev).String()
	return s.Publish(MainQueueName, ev, events.WithMetadata(map[string]string{
		MetadatakeyEventType: evName,
	}))
}
