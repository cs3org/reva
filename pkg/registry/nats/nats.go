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

	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	defaultBucket = "reva_registry"
	defaultTTL    = 30 * time.Second
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

	mu        sync.Mutex
	kv        jetstream.KeyValue
	nc        *nats.Conn
	connected bool
	pending   map[string]entry
	removed   map[string]struct{}
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
		pending:  map[string]entry{},
		removed:  map[string]struct{}{},
		keyIndex: map[string][2]string{},
		ctx:      ctx,
		cancel:   cancel,
	}
	return d, nil
}

func (d *driver) Add(service string, n registry.Node) error {
	e := entry{Service: service, ID: n.ID(), Address: n.Address(), Metadata: n.Metadata()}
	key := keyFor(service, n.ID())

	d.mu.Lock()
	d.keyIndex[key] = [2]string{service, n.ID()}
	connected, kv := d.connected, d.kv
	if !connected {
		d.pending[key] = e
		delete(d.removed, key)
		d.mu.Unlock()
		return nil
	}
	d.mu.Unlock()

	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(d.ctx, 5*time.Second)
	_, err = kv.Put(ctx, key, b)
	cancel()
	if err != nil {
		d.mu.Lock()
		d.pending[key] = e
		d.connected = false
		d.mu.Unlock()
	}
	return nil
}

func (d *driver) Remove(service, nodeID string) error {
	key := keyFor(service, nodeID)

	d.mu.Lock()
	connected, kv := d.connected, d.kv
	if !connected {
		d.removed[key] = struct{}{}
		delete(d.pending, key)
		d.mu.Unlock()
		return nil
	}
	d.mu.Unlock()

	ctx, cancel := context.WithTimeout(d.ctx, 5*time.Second)
	_ = kv.Delete(ctx, key)
	cancel()
	return nil
}

// Watch (re)connects if needed and streams the bucket, replaying existing keys
// first so the cache hydrates.
func (d *driver) Watch() (<-chan registry.Event, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	d.mu.Lock()
	kv := d.kv
	d.mu.Unlock()

	w, err := kv.WatchAll(d.ctx)
	if err != nil {
		return nil, err
	}
	out := make(chan registry.Event)
	go d.forward(w, out)
	return out, nil
}

func (d *driver) forward(w jetstream.KeyWatcher, out chan<- registry.Event) {
	defer close(out)
	defer w.Stop()
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
				out <- registry.Event{
					Type:    registry.EventRemove,
					Service: ids[0],
					Node:    registry.NewNode(ids[1], "", nil),
				}
			}
		}
	}
}

func (d *driver) ensureConnected() error {
	d.mu.Lock()
	if d.connected {
		d.mu.Unlock()
		return nil
	}
	d.mu.Unlock()
	return d.connect()
}

func (d *driver) connect() error {
	opts := []nats.Option{nats.Name("reva-registry")}
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
	d.connected = true
	d.mu.Unlock()

	d.flushPending()
	return nil
}

func (d *driver) flushPending() {
	d.mu.Lock()
	pending := d.pending
	removed := d.removed
	d.pending = map[string]entry{}
	d.removed = map[string]struct{}{}
	kv := d.kv
	d.mu.Unlock()

	ctx, cancel := context.WithTimeout(d.ctx, 10*time.Second)
	defer cancel()
	for key, e := range pending {
		if b, err := json.Marshal(e); err == nil {
			_, _ = kv.Put(ctx, key, b)
		}
	}
	for key := range removed {
		_ = kv.Delete(ctx, key)
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
