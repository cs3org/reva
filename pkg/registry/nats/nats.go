// Copyright 2018-2024 CERN
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

// Package nats is the shared registry backend on NATS JetStream KV. It is a
// registry.Driver: it writes through to the KV bucket and streams its changes;
// the cache, resolution and liveness live in registry.BaseRegistry. It never
// fails fast on an unreachable server: writes are queued and flushed on connect.
package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cs3org/reva/v3/pkg/logger"
	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog"
)

const (
	defaultBucket = "reva_registry"
	defaultTTL    = 30 * time.Second
	opTimeout     = 5 * time.Second
	flushTimeout  = 10 * time.Second
	reconnectWait = 2 * time.Second
	// maxReconnects bounds nats.go's own retries. On the last one it closes the
	// connection, which is what lets bound() dial a fresh one.
	maxReconnects = 10
)

func init() {
	registry.Register("nats", func(m map[string]any) (registry.Driver, error) {
		return New(m)
	})
}

// Config is the nats driver configuration.
type Config struct {
	Address string `mapstructure:"address"`
	Token   string `mapstructure:"token"`
	Bucket  string `mapstructure:"bucket"`
	TTL     string `mapstructure:"ttl"`
}

type entry struct {
	Service  string            `json:"service"`
	ID       string            `json:"id"`
	Address  string            `json:"address"`
	Metadata map[string]string `json:"metadata"`
}

type driver struct {
	cfg    Config
	bucket string
	ttl    time.Duration
	log    zerolog.Logger

	// dialMu serializes reconnects so concurrent callers share one connection.
	dialMu  sync.Mutex
	mu      sync.Mutex
	kv      jetstream.KeyValue
	nc      *nats.Conn
	pending map[string]entry
	removed map[string]struct{}
	// keyIndex maps a KV key back to its service+id for delete events.
	keyIndex map[string][2]string

	ctx    context.Context
	cancel context.CancelFunc
}

func New(m map[string]any) (registry.Driver, error) {
	var c Config
	if err := mapstructure.Decode(m, &c); err != nil {
		return nil, fmt.Errorf("nats registry: decoding config: %w", err)
	}
	if c.Address == "" {
		return nil, fmt.Errorf("nats registry: address is required")
	}
	bucket := c.Bucket
	if bucket == "" {
		bucket = defaultBucket
	}
	ttl := defaultTTL
	if c.TTL != "" {
		if d, err := time.ParseDuration(c.TTL); err == nil {
			ttl = d
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &driver{
		cfg:      c,
		bucket:   bucket,
		ttl:      ttl,
		log:      logger.New().With().Str("pkg", "registry").Str("driver", "nats").Logger(),
		pending:  map[string]entry{},
		removed:  map[string]struct{}{},
		keyIndex: map[string][2]string{},
		ctx:      ctx,
		cancel:   cancel,
	}
	return d, nil
}

// Add writes the node through to the bucket. A write that cannot be made is
// queued for the next flush and reported, never swallowed: an unreported queued
// write leaves the process healthy but invisible to its peers.
func (d *driver) Add(service string, n registry.Node) error {
	e := entry{Service: service, ID: n.ID(), Address: n.Address(), Metadata: n.Metadata()}
	key := keyFor(service, n.ID())
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}

	d.mu.Lock()
	d.keyIndex[key] = [2]string{service, n.ID()}
	d.mu.Unlock()

	kv, err := d.bound()
	if err != nil {
		d.queueAdd(key, e)
		return err
	}
	ctx, cancel := context.WithTimeout(d.ctx, opTimeout)
	_, err = kv.Put(ctx, key, b)
	cancel()
	if err != nil {
		d.queueAdd(key, e)
		return fmt.Errorf("nats registry: writing %q to bucket %s: %w", key, d.bucket, err)
	}
	return nil
}

func (d *driver) Remove(service, nodeID string) error {
	key := keyFor(service, nodeID)

	kv, err := d.bound()
	if err != nil {
		d.queueRemove(key)
		return err
	}
	ctx, cancel := context.WithTimeout(d.ctx, opTimeout)
	err = kv.Delete(ctx, key)
	cancel()
	if err != nil {
		d.queueRemove(key)
		return fmt.Errorf("nats registry: deleting %q from bucket %s: %w", key, d.bucket, err)
	}
	return nil
}

func (d *driver) queueAdd(key string, e entry) {
	d.mu.Lock()
	d.pending[key] = e
	delete(d.removed, key)
	d.mu.Unlock()
}

func (d *driver) queueRemove(key string) {
	d.mu.Lock()
	d.removed[key] = struct{}{}
	delete(d.pending, key)
	d.mu.Unlock()
}

// Watch (re)connects if needed and streams the bucket, replaying existing keys
// first so the cache hydrates.
func (d *driver) Watch() (<-chan registry.Event, error) {
	kv, err := d.bound()
	if err != nil {
		return nil, err
	}

	w, err := kv.WatchAll(d.ctx)
	if err != nil {
		return nil, err
	}
	d.log.Info().Str("bucket", d.bucket).Msg("registry watch established, replaying bucket")
	out := make(chan registry.Event)
	go d.forward(w, out)
	return out, nil
}

func (d *driver) forward(w jetstream.KeyWatcher, out chan<- registry.Event) {
	defer close(out)
	defer w.Stop()
	defer d.log.Warn().Str("bucket", d.bucket).Msg("registry watch closed; cache is now stale until it re-establishes")
	for {
		select {
		case <-d.ctx.Done():
			return
		case ke, ok := <-w.Updates():
			if !ok {
				return
			}
			if ke == nil {
				continue
			}
			switch ke.Operation() {
			case jetstream.KeyValuePut:
				var e entry
				if err := json.Unmarshal(ke.Value(), &e); err != nil {
					continue
				}
				d.mu.Lock()
				d.keyIndex[ke.Key()] = [2]string{e.Service, e.ID}
				d.mu.Unlock()
				out <- registry.Event{
					Type:    registry.EventAdd,
					Service: e.Service,
					Node:    registry.NewNode(e.ID, e.Address, e.Metadata),
				}
			case jetstream.KeyValueDelete, jetstream.KeyValuePurge:
				d.mu.Lock()
				ids, known := d.keyIndex[ke.Key()]
				delete(d.keyIndex, ke.Key())
				d.mu.Unlock()
				if !known {
					continue
				}
				d.log.Debug().Str("service", ids[0]).Str("node", ids[1]).
					Str("op", ke.Operation().String()).Msg("registry entry removed (explicit delete or ttl expiry)")
				out <- registry.Event{
					Type:    registry.EventRemove,
					Service: ids[0],
					Node:    registry.NewNode(ids[1], "", nil),
				}
			}
		}
	}
}

// bound returns the bucket handle when the connection can carry a request. It
// dials on first use and after a close, but leaves a connection that is merely
// reconnecting alone, since nats.go is still retrying it. Connection state is
// always read from the live connection, never from a cached flag that a failed
// write could latch.
func (d *driver) bound() (jetstream.KeyValue, error) {
	d.mu.Lock()
	nc, kv := d.nc, d.kv
	d.mu.Unlock()

	switch {
	case nc == nil || nc.IsClosed():
		if err := d.connect(); err != nil {
			return nil, fmt.Errorf("nats registry: connecting to %s: %w", d.cfg.Address, err)
		}
		d.mu.Lock()
		kv = d.kv
		d.mu.Unlock()
		return kv, nil
	case !nc.IsConnected():
		return nil, fmt.Errorf("nats registry: reconnecting to %s", d.cfg.Address)
	case kv == nil:
		return nil, fmt.Errorf("nats registry: bucket %s is not bound", d.bucket)
	}
	return kv, nil
}

func (d *driver) connect() error {
	d.dialMu.Lock()
	defer d.dialMu.Unlock()

	d.mu.Lock()
	current := d.nc
	d.mu.Unlock()
	if current != nil && !current.IsClosed() {
		return nil
	}

	opts := []nats.Option{
		nats.Name("reva-registry"),
		nats.MaxReconnects(maxReconnects),
		nats.ReconnectWait(reconnectWait),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			d.log.Warn().Err(err).Str("address", d.cfg.Address).Msg("registry nats disconnected; writes now queue in memory")
		}),
		nats.ReconnectHandler(func(*nats.Conn) {
			d.log.Info().Str("address", d.cfg.Address).Msg("registry nats reconnected")
			d.flushPending()
		}),
		nats.ClosedHandler(func(_ *nats.Conn) {
			d.log.Error().Str("address", d.cfg.Address).Msg("registry nats connection closed; peers are invisible until it redials")
		}),
	}
	if d.cfg.Token != "" {
		opts = append(opts, nats.Token(d.cfg.Token))
	}
	nc, err := nats.Connect(d.cfg.Address, opts...)
	if err != nil {
		return err
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return err
	}
	ctx, cancel := context.WithTimeout(d.ctx, 10*time.Second)
	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      d.bucket,
		Description: "reva service registry",
		History:     1,
		TTL:         d.ttl,
	})
	cancel()
	if err != nil {
		nc.Close()
		return err
	}

	d.mu.Lock()
	d.nc = nc
	d.kv = kv
	d.mu.Unlock()

	d.flushPending()
	return nil
}

// flushPending drains the writes that queued while the bucket was unreachable.
// A write that fails again is queued once more rather than dropped.
func (d *driver) flushPending() {
	d.mu.Lock()
	kv := d.kv
	if kv == nil || (len(d.pending) == 0 && len(d.removed) == 0) {
		d.mu.Unlock()
		return
	}
	pending := d.pending
	removed := d.removed
	d.pending = map[string]entry{}
	d.removed = map[string]struct{}{}
	d.mu.Unlock()

	d.log.Info().Int("adds", len(pending)).Int("removes", len(removed)).Msg("flushing queued registry writes")
	ctx, cancel := context.WithTimeout(d.ctx, flushTimeout)
	defer cancel()
	for key, e := range pending {
		b, err := json.Marshal(e)
		if err != nil {
			continue
		}
		if _, err := kv.Put(ctx, key, b); err != nil {
			d.queueAdd(key, e)
		}
	}
	for key := range removed {
		if err := kv.Delete(ctx, key); err != nil {
			d.queueRemove(key)
		}
	}
}

func (d *driver) Close() {
	d.cancel()
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.nc != nil {
		d.nc.Close()
	}
}

func keyFor(service, id string) string {
	return sanitize(service) + "." + sanitize(id)
}

// sanitize maps a string to NATS-legal key tokens.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, s)
}
