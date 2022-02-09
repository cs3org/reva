package main

import (
	"fmt"
	"log"

	"github.com/cs3org/reva/pkg/events"
	"github.com/cs3org/reva/pkg/events/server"
)

// starts a server and listens for all events
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

	group := ""
	c, err := events.Consume(s, group, events.ShareCreated{})
	if err != nil {
		fmt.Println("consumer", group, "can't consume", err)
		return
	}

	fmt.Println("consumer waiting")
	for {
		b := <-c

		// proposed usage
		switch v := b.(type) {
		case events.ShareCreated:
			fmt.Printf("%s) Share created: %+v\n", group, v)
		default:
			fmt.Printf("%s) Unregistered event: %+v\n", group, v)
		}
	}
}
