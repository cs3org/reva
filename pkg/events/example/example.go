package main

import (
	"log"
	"time"

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
	c, err := server.NewNatsStream()
	if err != nil {
		log.Fatal(err)
	}
	return c

}
