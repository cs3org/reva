package server

import (
	"github.com/asim/go-micro/plugins/events/nats/v4"
	"github.com/nats-io/nats-streaming-server/server"
	"go-micro.dev/v4/events"
)

// RunNatsServer runs the nats streaming server
func RunNatsServer() error {
	// TODO: configurable options
	_, err := server.RunServerWithOpts(nil, nil)
	return err
}

// NewNatsStream returns a streaming client used by `Consume` and `Publish` methods
func NewNatsStream() (events.Stream, error) {
	// TODO: configurable options
	return nats.NewStream(nats.Address("127.0.0.1:4222"), nats.ClusterID("test-cluster"))
}
