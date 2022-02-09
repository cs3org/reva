package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/events"
	"github.com/cs3org/reva/pkg/events/server"
	microevents "go-micro.dev/v4/events"
)

func main() {
	// start server
	err := server.RunNatsServer()
	if err != nil {
		log.Fatal(err)
	}

	// get client
	s, err := server.NewNatsStream()
	if err != nil {
		log.Fatal(err)
	}

	// needed for syncing
	wg := &sync.WaitGroup{}

	// consumer a has two instances - it is supposed to get only one event
	wg.Add(1)
	go Consumer(s, "a", wg)
	wg.Add(1)
	go Consumer(s, "a", wg)

	// consumers b and c are supposed to get the event also
	wg.Add(1)
	go Consumer(s, "b", wg)
	wg.Add(1)
	go Consumer(s, "c", wg)

	// wait for consumer registration
	wg.Wait()

	// publish an event
	sc := events.ShareCreated{
		//Sharer: "userA",
		//Sharee:   "userB",
		ItemID: &provider.ResourceId{
			StorageId: "storageA",
			OpaqueId:  "opaqueB",
		},
	}
	if err := events.Publish(sc, s); err != nil {
		log.Fatal(err)
	}

	// publish another event
	sc = events.ShareCreated{
		//SharerID: "user34",
		//Sharee:   "user12732",
		ItemID: &provider.ResourceId{
			StorageId: "storage44",
			OpaqueId:  "opaque231",
		},
	}
	if err := events.Publish(sc, s); err != nil {
		log.Fatal(err)
	}

	// wait for consumers to log
	time.Sleep(3 * time.Second)

}

// Consumer consumes from queue
func Consumer(s microevents.Stream, group string, wg *sync.WaitGroup) {
	c, err := events.Consume(s, group, events.ShareCreated{})
	if err != nil {
		wg.Done()
		fmt.Println("consumer", group, "can't consume", err)
		return
	}

	wg.Done()
	fmt.Printf("%s) consumer waiting\n", group)
	for {
		b := <-c

		// proposed usage
		switch v := b.(type) {
		case events.ShareCreated:
			fmt.Printf("%s) Share created: %+v\n", group, v)
		default:
			fmt.Printf("%s) Unregistered event: %T, %+v\n", group, v, v)
		}
	}
}
