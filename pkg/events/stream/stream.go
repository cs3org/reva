// Package stream provides streaming clients used by `Consume` and `Publish` methods
package stream

import (
	"encoding/json"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/cs3org/reva/v2/pkg/logger"
	"go-micro.dev/v4/events"

	"github.com/go-micro/plugins/v4/events/natsjs"
)

// Nats returns a nats streaming client
// retries exponentially to connect to a nats server
func Nats(opts ...natsjs.Option) (events.Stream, error) {
	b := backoff.NewExponentialBackOff()
	var stream events.Stream
	o := func() error {
		n := b.NextBackOff()
		s, err := natsjs.NewStream(opts...)
		if err != nil && n > time.Second {
			logger.New().Error().Err(err).Msgf("can't connect to nats (jetstream) server, retrying in %s", n)
		}
		stream = s
		return err
	}

	err := backoff.Retry(o, b)
	return stream, err
}

// Chan is a channel based streaming clients
// Useful for tests or in memory applications
type Chan [2]chan interface{}

// Publish not implemented atm
func (ch Chan) Publish(_ string, msg interface{}, _ ...events.PublishOption) error {
	go func() {
		ch[0] <- msg
	}()
	return nil
}

// Consume implementation
func (ch Chan) Consume(_ string, _ ...events.ConsumeOption) (<-chan events.Event, error) {
	evch := make(chan events.Event)
	go func() {
		e := <-ch[1]
		b, _ := json.Marshal(e)
		evch <- events.Event{Payload: b}
	}()
	return evch, nil
}
