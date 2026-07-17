// Copyright 2018-2026 CERN
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

package backends

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cs3org/reva/v3/pkg/notifications/model"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
)

const defaultNotificationSubject = "reva-notifications.send"

// NATSConfig configures the NATS notification backend and listener.
type NATSConfig struct {
	Address string `mapstructure:"address"`
	Token   string `mapstructure:"token"`
	Stream  string `mapstructure:"stream"`
	Subject string `mapstructure:"subject"`
	Durable string `mapstructure:"durable"`
	Queue   string `mapstructure:"queue"`
}

func (c NATSConfig) subject() string {
	if c.Subject != "" {
		return c.Subject
	}
	return defaultNotificationSubject
}

func (c NATSConfig) stream() string {
	if c.Stream != "" {
		return c.Stream
	}
	return "reva-notifications"
}

func (c NATSConfig) durable() string {
	if c.Durable != "" {
		return c.Durable
	}
	return "reva-notifications-handler"
}

func (c NATSConfig) queue() string {
	if c.Queue != "" {
		return c.Queue
	}
	return "reva-notifications-workers"
}

// NATSBackend publishes accepted notifications to JetStream.
type NATSBackend struct {
	nc   *nats.Conn
	js   nats.JetStreamContext
	conf NATSConfig
	log  zerolog.Logger
}

// NewNATSBackend connects to NATS and ensures the notification stream exists.
func NewNATSBackend(conf NATSConfig, log zerolog.Logger) (*NATSBackend, error) {
	nc, err := utils.ConnectToNats(conf.Address, conf.Token, log)
	if err != nil {
		return nil, err
	}

	js, err := nc.JetStream(nats.PublishAsyncMaxPending(256))
	if err != nil {
		_ = nc.Drain()
		return nil, err
	}

	if err := ensureNotificationStream(js, conf); err != nil {
		_ = nc.Drain()
		return nil, err
	}

	return &NATSBackend{
		nc:   nc,
		js:   js,
		conf: conf,
		log:  log,
	}, nil
}

// Publish implements Backend.
func (b *NATSBackend) Publish(_ context.Context, envelope model.Envelope) error {
	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	ack, err := b.js.Publish(b.conf.subject(), data)
	if err != nil {
		b.log.Error().
			Err(err).
			Str("notification_id", envelope.ID).
			Str("event_type", envelope.EventType).
			Int("recipients", len(envelope.Recipients)).
			Str("stream", b.conf.stream()).
			Str("subject", b.conf.subject()).
			Msg("notifications: failed to publish event to nats")
		return err
	}

	event := b.log.Info().
		Str("notification_id", envelope.ID).
		Str("event_type", envelope.EventType).
		Int("recipients", len(envelope.Recipients)).
		Str("stream", b.conf.stream()).
		Str("subject", b.conf.subject())
	if ack != nil {
		event = event.Str("ack_stream", ack.Stream).Uint64("ack_sequence", ack.Sequence)
	}
	event.Msg("notifications: published event to nats")
	return nil
}

// Close drains the NATS connection.
func (b *NATSBackend) Close() error {
	if b == nil || b.nc == nil {
		return nil
	}
	return b.nc.Drain()
}

// NATSListener consumes notification envelopes from JetStream.
type NATSListener struct {
	nc   *nats.Conn
	js   nats.JetStreamContext
	conf NATSConfig
	sub  *nats.Subscription
	log  zerolog.Logger
}

// NewNATSListener connects to NATS and ensures the notification stream exists.
func NewNATSListener(conf NATSConfig, log zerolog.Logger) (*NATSListener, error) {
	nc, err := utils.ConnectToNats(conf.Address, conf.Token, log)
	if err != nil {
		return nil, err
	}

	js, err := nc.JetStream()
	if err != nil {
		_ = nc.Drain()
		return nil, err
	}

	if err := ensureNotificationStream(js, conf); err != nil {
		_ = nc.Drain()
		return nil, err
	}

	return &NATSListener{
		nc:   nc,
		js:   js,
		conf: conf,
		log:  log,
	}, nil
}

// Start subscribes to the notification stream. Messages are acked only when the
// handler returns nil.
func (l *NATSListener) Start(ctx context.Context, handler func(context.Context, model.Envelope) error) error {
	if l == nil || l.js == nil {
		return fmt.Errorf("nats listener is not configured")
	}

	sub, err := l.js.QueueSubscribe(
		l.conf.subject(),
		l.conf.queue(),
		func(msg *nats.Msg) {
			var envelope model.Envelope
			if err := json.Unmarshal(msg.Data, &envelope); err != nil {
				l.log.Error().
					Err(err).
					Str("stream", l.conf.stream()).
					Str("subject", l.conf.subject()).
					Msg("notifications: failed to decode nats message")
				if termErr := msg.Term(); termErr != nil {
					l.log.Error().Err(termErr).Msg("notifications: failed to terminate undecodable nats message")
				}
				return
			}

			l.log.Info().
				Str("notification_id", envelope.ID).
				Str("event_type", envelope.EventType).
				Int("recipients", len(envelope.Recipients)).
				Str("stream", l.conf.stream()).
				Str("subject", l.conf.subject()).
				Msg("notifications: received event from nats")

			if err := handler(ctx, envelope); err != nil {
				l.log.Error().
					Err(err).
					Str("notification_id", envelope.ID).
					Str("event_type", envelope.EventType).
					Int("recipients", len(envelope.Recipients)).
					Str("stream", l.conf.stream()).
					Str("subject", l.conf.subject()).
					Msg("notifications: failed to handle nats event")
				if nakErr := msg.Nak(); nakErr != nil {
					l.log.Error().
						Err(nakErr).
						Str("notification_id", envelope.ID).
						Str("event_type", envelope.EventType).
						Msg("notifications: failed to nak nats event")
				}
				return
			}

			if ackErr := msg.Ack(); ackErr != nil {
				l.log.Error().
					Err(ackErr).
					Str("notification_id", envelope.ID).
					Str("event_type", envelope.EventType).
					Msg("notifications: failed to ack nats event")
			}
		},
		nats.Durable(l.conf.durable()),
		nats.ManualAck(),
	)
	if err != nil {
		return err
	}

	l.sub = sub
	return nil
}

// Close drains the NATS listener connection.
func (l *NATSListener) Close() error {
	if l == nil || l.nc == nil {
		return nil
	}
	return l.nc.Drain()
}

func ensureNotificationStream(js nats.JetStreamContext, conf NATSConfig) error {
	if _, err := js.StreamInfo(conf.stream()); err == nil {
		return nil
	}

	_, err := js.AddStream(&nats.StreamConfig{
		Name:     conf.stream(),
		Subjects: []string{conf.subject()},
	})
	return err
}
