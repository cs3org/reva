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

package notifications

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cs3org/reva/v3/pkg/notification/utils"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
)

const defaultNotificationSubject = "reva-notifications.send"

// NATSConfig configures the NATS notification backend and listener.
type NATSConfig struct {
	Address string
	Token   string
	Stream  string
	Subject string
	Durable string
	Queue   string
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
	}, nil
}

// Publish implements Backend.
func (b *NATSBackend) Publish(_ context.Context, envelope Envelope) error {
	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	_, err = b.js.Publish(b.conf.subject(), data)
	return err
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
	}, nil
}

// Start subscribes to the notification stream. Messages are acked only when the
// handler returns nil.
func (l *NATSListener) Start(ctx context.Context, handler func(context.Context, Envelope) error) error {
	if l == nil || l.js == nil {
		return fmt.Errorf("nats listener is not configured")
	}

	sub, err := l.js.QueueSubscribe(
		l.conf.subject(),
		l.conf.queue(),
		func(msg *nats.Msg) {
			var envelope Envelope
			if err := json.Unmarshal(msg.Data, &envelope); err != nil {
				_ = msg.Term()
				return
			}

			if err := handler(ctx, envelope); err != nil {
				_ = msg.Nak()
				return
			}

			_ = msg.Ack()
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
