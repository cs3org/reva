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

// Package nats implements the rjobs.Store on top of NATS JetStream. The work
// queue (Enqueue/Claim/Complete/Fail) is a JetStream stream with a durable
// pull consumer and explicit acks; the per-job next-fire state used by the
// scheduler lives in a JetStream key-value bucket, whose revision-based
// compare-and-set gives the atomic next-fire advance that guarantees a
// periodic job fires once even with more than one scheduler.
package nats

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/cs3org/reva/v3/pkg/notification/utils"
	"github.com/cs3org/reva/v3/pkg/rjobs"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// fetchWait bounds a single Claim fetch. A quiet queue returns to the claim
// loop after this interval so it can re-check the outer context, rather than
// blocking on the server indefinitely.
const fetchWait = 5 * time.Second

// Options configures the NATS-backed store.
type Options struct {
	// Address is the NATS server address.
	Address string
	// Token is the authentication token, if any.
	Token string
	// Prefix namespaces the stream, consumer and KV bucket.
	Prefix string
	// AckWait is the visibility timeout: how long a claimed run may be in
	// flight before JetStream redelivers it. Defaults to one minute.
	AckWait time.Duration
}

type store struct {
	nc     *nats.Conn
	js     nats.JetStreamContext
	sub    *nats.Subscription
	kv     nats.KeyValue
	prefix string
	log    zerolog.Logger

	// inflight tracks the NATS message backing each claimed run, so that
	// Complete and Fail can ack or nak the right message.
	mu       sync.Mutex
	inflight map[rjobs.RunID]*nats.Msg
}

// scheduleState is what we keep per leader-scoped periodic job in the KV
// bucket, so the next-fire survives restarts and can be advanced atomically.
type scheduleState struct {
	Interval time.Duration `json:"interval"`
	Next     time.Time     `json:"next"`
}

// New connects to NATS and sets up the stream, consumer and KV bucket.
func New(ctx context.Context, opts Options) (rjobs.Store, error) {
	if opts.Prefix == "" {
		opts.Prefix = "reva-jobs"
	}
	if opts.AckWait <= 0 {
		opts.AckWait = time.Minute
	}

	log := *zerolog.Ctx(ctx)

	nc, err := utils.ConnectToNats(opts.Address, opts.Token, log)
	if err != nil {
		return nil, err
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, errors.Wrap(err, "rjobs: jetstream initialization failed")
	}

	s := &store{
		nc:       nc,
		js:       js,
		prefix:   opts.Prefix,
		log:      log,
		inflight: make(map[rjobs.RunID]*nats.Msg),
	}

	if err := s.setup(opts.AckWait); err != nil {
		nc.Close()
		return nil, err
	}

	return s, nil
}

func (s *store) streamName() string   { return s.prefix + "-runs" }
func (s *store) consumerName() string { return s.prefix + "-worker" }
func (s *store) subject() string      { return s.prefix + ".runs" }
func (s *store) bucketName() string   { return s.prefix + "-schedule" }

func (s *store) setup(ackWait time.Duration) error {
	if _, err := s.js.AddStream(&nats.StreamConfig{
		Name:      s.streamName(),
		Subjects:  []string{s.subject()},
		Retention: nats.WorkQueuePolicy,
	}); err != nil {
		return errors.Wrap(err, "rjobs: runs stream creation failed")
	}

	if _, err := s.js.AddConsumer(s.streamName(), &nats.ConsumerConfig{
		Durable:       s.consumerName(),
		AckPolicy:     nats.AckExplicitPolicy,
		AckWait:       ackWait,
		MaxAckPending: -1,
	}); err != nil {
		return errors.Wrap(err, "rjobs: runs consumer creation failed")
	}

	sub, err := s.js.PullSubscribe(s.subject(), s.consumerName(), nats.Bind(s.streamName(), s.consumerName()))
	if err != nil {
		return errors.Wrap(err, "rjobs: pull subscription failed")
	}
	s.sub = sub

	kv, err := s.js.CreateKeyValue(&nats.KeyValueConfig{Bucket: s.bucketName()})
	if err != nil {
		return errors.Wrap(err, "rjobs: schedule bucket creation failed")
	}
	s.kv = kv

	return nil
}

func (s *store) track(id rjobs.RunID, msg *nats.Msg) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inflight[id] = msg
}

func (s *store) untrack(id rjobs.RunID) (*nats.Msg, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msg, ok := s.inflight[id]
	if ok {
		delete(s.inflight, id)
	}
	return msg, ok
}

func (s *store) Enqueue(ctx context.Context, run rjobs.Run) (rjobs.RunID, error) {
	if run.ID == "" {
		run.ID = rjobs.RunID(nats.NewInbox())
	}
	if run.Attempt == 0 {
		run.Attempt = 1
	}
	if run.EnqueuedAt.IsZero() {
		run.EnqueuedAt = time.Now()
	}

	payload, err := json.Marshal(run)
	if err != nil {
		return "", errors.Wrap(err, "rjobs: marshalling run failed")
	}

	pubOpts := []nats.PubOpt{nats.Context(ctx)}
	if run.IdempotencyKey != "" {
		// JetStream dedups messages carrying the same Nats-Msg-Id within the
		// stream's duplicate window, collapsing repeated enqueues.
		pubOpts = append(pubOpts, nats.MsgId(run.IdempotencyKey))
	}

	if _, err := s.js.Publish(s.subject(), payload, pubOpts...); err != nil {
		return "", errors.Wrap(err, "rjobs: publishing run failed")
	}
	return run.ID, nil
}

func (s *store) Claim(ctx context.Context) (rjobs.Run, error) {
	for {
		if err := ctx.Err(); err != nil {
			return rjobs.Run{}, err
		}

		fetchCtx, cancel := context.WithTimeout(ctx, fetchWait)
		msgs, err := s.sub.Fetch(1, nats.Context(fetchCtx))
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return rjobs.Run{}, ctx.Err()
			}
			if errors.Is(err, nats.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
				continue
			}
			return rjobs.Run{}, errors.Wrap(err, "rjobs: fetching run failed")
		}
		if len(msgs) == 0 {
			continue
		}

		msg := msgs[0]
		var run rjobs.Run
		if err := json.Unmarshal(msg.Data, &run); err != nil {
			// a run we cannot decode is poison; drop it so it does not block
			// the queue, and keep going.
			s.log.Error().Err(err).Msg("rjobs: dropping undecodable run")
			_ = msg.Term()
			continue
		}

		s.track(run.ID, msg)
		return run, nil
	}
}

func (s *store) Complete(ctx context.Context, id rjobs.RunID) error {
	msg, ok := s.untrack(id)
	if !ok {
		return errors.Errorf("rjobs: no in-flight run %q to complete", id)
	}
	if err := msg.AckSync(nats.Context(ctx)); err != nil {
		return errors.Wrap(err, "rjobs: acking run failed")
	}
	return nil
}

func (s *store) Fail(ctx context.Context, id rjobs.RunID, retryAfter time.Duration) error {
	msg, ok := s.untrack(id)
	if !ok {
		return errors.Errorf("rjobs: no in-flight run %q to fail", id)
	}
	// NakWithDelay redelivers the run after retryAfter, preserving the
	// at-least-once contract.
	if err := msg.NakWithDelay(retryAfter); err != nil {
		return errors.Wrap(err, "rjobs: nak-ing run failed")
	}
	return nil
}

func (s *store) RegisterScheduled(ctx context.Context, job string, schedule rjobs.Schedule, next time.Time) error {
	entry, err := s.kv.Get(job)
	switch {
	case err == nil:
		// An entry exists. Keep it as-is on a restart so the cadence is not
		// reset, UNLESS the configured interval changed: then adopt the new
		// interval and the recomputed next-fire so a schedule change in config
		// takes effect.
		var cur scheduleState
		if uerr := json.Unmarshal(entry.Value(), &cur); uerr != nil {
			return errors.Wrap(uerr, "rjobs: reading schedule state failed")
		}
		if cur.Interval == schedule.Interval() {
			return nil
		}
		st := scheduleState{Interval: schedule.Interval(), Next: next}
		data, merr := json.Marshal(st)
		if merr != nil {
			return errors.Wrap(merr, "rjobs: marshalling schedule state failed")
		}
		if _, uerr := s.kv.Update(job, data, entry.Revision()); uerr != nil {
			// another process updated it first; its write wins, which is fine.
			return nil
		}
		return nil
	case errors.Is(err, nats.ErrKeyNotFound):
		// fall through to create below.
	default:
		return errors.Wrap(err, "rjobs: reading schedule state failed")
	}

	st := scheduleState{Interval: schedule.Interval(), Next: next}
	data, err := json.Marshal(st)
	if err != nil {
		return errors.Wrap(err, "rjobs: marshalling schedule state failed")
	}
	if _, err := s.kv.Create(job, data); err != nil {
		// a concurrent scheduler created it first, which is fine.
		if errors.Is(err, nats.ErrKeyExists) {
			return nil
		}
		return errors.Wrap(err, "rjobs: creating schedule state failed")
	}
	return nil
}

func (s *store) DueScheduled(ctx context.Context, now time.Time) ([]rjobs.ScheduledRun, error) {
	keys, err := s.kv.Keys()
	if err != nil {
		if errors.Is(err, nats.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "rjobs: listing schedule state failed")
	}

	var due []rjobs.ScheduledRun
	for _, job := range keys {
		entry, err := s.kv.Get(job)
		if err != nil {
			if errors.Is(err, nats.ErrKeyNotFound) {
				continue
			}
			return nil, errors.Wrap(err, "rjobs: reading schedule state failed")
		}

		var st scheduleState
		if err := json.Unmarshal(entry.Value(), &st); err != nil {
			s.log.Error().Err(err).Str("job", job).Msg("rjobs: dropping undecodable schedule state")
			continue
		}
		if st.Next.After(now) {
			continue
		}

		// Advance to the next fire after now. The Update is conditioned on the
		// revision we just read, so if another scheduler advances it first our
		// update fails and we skip the job: it fires exactly once.
		next := st.Next.Add(st.Interval)
		for !next.After(now) {
			next = next.Add(st.Interval)
		}
		st.Next = next
		data, err := json.Marshal(st)
		if err != nil {
			return nil, errors.Wrap(err, "rjobs: marshalling schedule state failed")
		}
		if _, err := s.kv.Update(job, data, entry.Revision()); err != nil {
			// lost the race to another scheduler; that scheduler owns this
			// tick.
			continue
		}
		due = append(due, rjobs.ScheduledRun{Job: job, Next: next})
	}
	return due, nil
}

func (s *store) Close(ctx context.Context) error {
	if s.sub != nil {
		_ = s.sub.Drain()
	}
	return s.nc.Drain()
}
