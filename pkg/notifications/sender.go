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

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/notifications/backends"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

// SenderConfig configures notification submission from services that cannot yet
// call the generated gateway SendNotification RPC.
type SenderConfig struct {
	NATS backends.NATSConfig `mapstructure:"nats"`

	// Keep the flat NATS keys accepted by existing configs while the backend
	// config is being moved under a nested notifications.nats section.
	NATSAddress string `mapstructure:"nats_address"`
	NATSToken   string `mapstructure:"nats_token"`
}

func (c *SenderConfig) ApplyDefaults() {
	if c.NATS.Address == "" {
		c.NATS.Address = c.NATSAddress
	}
	if c.NATS.Token == "" {
		c.NATS.Token = c.NATSToken
	}
}

// NewSender creates a SendService and close function from service config. A nil
// sender means notifications are not configured for that service.
func NewSender(ctx context.Context, m map[string]any) (*SendService, func() error, error) {
	if len(m) == 0 {
		return nil, nil, nil
	}

	var c SenderConfig
	if err := cfg.Decode(m, &c); err != nil {
		return nil, nil, err
	}
	c.ApplyDefaults()
	if c.NATS.Address == "" {
		return nil, nil, nil
	}

	backend, err := backends.NewNATSBackend(c.NATS, *appctx.GetLogger(ctx))
	if err != nil {
		return nil, nil, err
	}

	return NewSendService(backend, NoopRateLimiter{}), backend.Close, nil
}
