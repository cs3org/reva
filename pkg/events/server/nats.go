package server

import (
	"github.com/asim/go-micro/plugins/events/nats/v4"
	"go-micro.dev/v4/events"

	stanServer "github.com/nats-io/nats-streaming-server/server"
)

// RunNatsServer runs the nats streaming server
func RunNatsServer(opts ...Option) error {
	natsOpts := stanServer.DefaultNatsServerOptions
	stanOpts := stanServer.GetDefaultOptions()

	for _, o := range opts {
		o(&natsOpts, stanOpts)
	}
	_, err := stanServer.RunServerWithOpts(stanOpts, &natsOpts)
	return err
}

// NewNatsStream returns a streaming client used by `Consume` and `Publish` methods
func NewNatsStream(opts ...nats.Option) (events.Stream, error) {
	return nats.NewStream(opts...)
}
