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
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"slices"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/notifications/accumulation"
	"github.com/cs3org/reva/v3/pkg/notifications/handlers"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
)

const defaultMaxRenderedItems = 10

// PreferenceResolver narrows the configured handler set for a recipient set.
type PreferenceResolver interface {
	ResolveHandlers(ctx context.Context, envelope model.Envelope, handlers []string) ([]string, error)
}

// NoopPreferenceResolver applies no recipient preference changes.
type NoopPreferenceResolver struct{}

// ResolveHandlers implements PreferenceResolver.
func (NoopPreferenceResolver) ResolveHandlers(_ context.Context, _ model.Envelope, handlers []string) ([]string, error) {
	return append([]string(nil), handlers...), nil
}

// Worker handles notification envelopes consumed from NATS.
type Worker struct {
	store      accumulation.Store
	dispatcher *handlers.Dispatcher
	rules      map[string]model.EventRule
	prefs      PreferenceResolver
	ownerID    string

	leaseDuration    time.Duration
	maxRenderedItems int
	now              func() time.Time

	mu     sync.Mutex
	timers map[string]*time.Timer
}

// WorkerConfig configures a notification worker.
type WorkerConfig struct {
	OwnerID          string
	EventRules       map[string]model.EventRule
	Preferences      PreferenceResolver
	LeaseDuration    time.Duration
	MaxRenderedItems int
}

// NewWorker creates a notification worker.
func NewWorker(store accumulation.Store, dispatcher *handlers.Dispatcher, conf WorkerConfig) (*Worker, error) {
	if dispatcher == nil {
		return nil, errors.New("notification dispatcher is required")
	}
	if conf.OwnerID == "" {
		return nil, errors.New("worker owner id is required")
	}
	if conf.Preferences == nil {
		conf.Preferences = NoopPreferenceResolver{}
	}
	if conf.LeaseDuration <= 0 {
		conf.LeaseDuration = 5 * time.Minute
	}
	if conf.MaxRenderedItems <= 0 {
		conf.MaxRenderedItems = defaultMaxRenderedItems
	}

	return &Worker{
		store:            store,
		dispatcher:       dispatcher,
		rules:            conf.EventRules,
		prefs:            conf.Preferences,
		ownerID:          conf.OwnerID,
		leaseDuration:    conf.LeaseDuration,
		maxRenderedItems: conf.MaxRenderedItems,
		now:              time.Now,
		timers:           make(map[string]*time.Timer),
	}, nil
}

// Handle handles one notification envelope.
func (w *Worker) Handle(ctx context.Context, envelope model.Envelope) error {
	resolved, err := w.resolve(ctx, envelope)
	if err != nil {
		return err
	}
	if len(resolved.Handlers) == 0 {
		return nil
	}

	switch resolved.Type {
	case model.TypeDirect:
		return w.dispatcher.Dispatch(ctx, resolved)
	case model.TypeAccumulated:
		if w.store == nil {
			return errors.New("accumulation store is required for accumulated notifications")
		}

		recipientEnvelopes, err := perRecipientAccumulationEnvelopes(resolved)
		if err != nil {
			return err
		}

		log := appctx.GetLogger(ctx)
		for _, recipientEnvelope := range recipientEnvelopes {
			group, err := w.store.Add(ctx, recipientEnvelope, w.now())
			if err != nil {
				return err
			}
			ev := log.Info().
				Str("worker", w.ownerID).
				Str("dedup_key", group.DedupKey).
				Str("event_type", recipientEnvelope.EventType).
				Int("item_count", group.ItemCount).
				Time("flush_after", group.FlushAfter)
			if group.ItemCount == 1 {
				ev.Msg("notifications: opened accumulator")
			} else {
				ev.Msg("notifications: added event to accumulator")
			}
			if err := w.resume(ctx, group); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported notification type %q", envelope.Type)
	}
}

func (w *Worker) resolve(ctx context.Context, envelope model.Envelope) (model.Envelope, error) {
	rule, ok := w.rules[envelope.EventType]
	if !ok {
		return model.Envelope{}, fmt.Errorf("notification event type %q is not configured", envelope.EventType)
	}

	resolved := envelope
	resolved.Type = rule.Type
	resolved.Accumulation = rule.Accumulation
	resolved.TemplateData = cloneMap(envelope.TemplateData)

	handlersToRun := handlerNames(rule.Handlers)
	filteredHandlers, err := w.prefs.ResolveHandlers(ctx, resolved, handlersToRun)
	if err != nil {
		return model.Envelope{}, err
	}
	resolved.Handlers = filteredHandlers

	if emailRule, ok := rule.Handlers[handlers.EmailHandlerName]; ok {
		resolved.TemplateName = emailRule.TemplateName
	}

	switch resolved.Type {
	case model.TypeDirect:
		return resolved, nil
	case model.TypeAccumulated:
		if resolved.Accumulation.WindowSeconds <= 0 {
			return model.Envelope{}, errors.New("accumulated notification rule requires a positive accumulation window")
		}
		dedupKey, err := renderDedupKey(rule.DedupKeyTemplate, resolved)
		if err != nil {
			return model.Envelope{}, err
		}
		resolved.DedupKey = dedupKey
		return resolved, nil
	default:
		return model.Envelope{}, fmt.Errorf("unsupported notification delivery type %q", resolved.Type)
	}
}

// resume takes over an accumulation: it leases it and then either flushes now
// (its window elapsed or it hit its item cap) or schedules a flush for when the
// window elapses.
func (w *Worker) resume(ctx context.Context, acc *accumulation.Group) error {
	now := w.now()
	leaseUntil := acc.FlushAfter.Add(w.leaseDuration)
	if leaseUntil.Before(now.Add(w.leaseDuration)) {
		leaseUntil = now.Add(w.leaseDuration)
	}

	acquired, err := w.store.AcquireLease(ctx, acc.DedupKey, w.ownerID, leaseUntil, now)
	if err != nil {
		return err
	}

	log := appctx.GetLogger(ctx)
	if !acquired {
		log.Info().Str("worker", w.ownerID).Str("dedup_key", acc.DedupKey).Msg("notifications: accumulator lease held by another worker, not flushing here")
		return nil
	}
	log.Info().Str("worker", w.ownerID).Str("dedup_key", acc.DedupKey).Msg("notifications: holding accumulator lease")

	if acc.ItemCount >= acc.MaxItems || !acc.FlushAfter.After(now) {
		return w.Flush(ctx, acc.DedupKey)
	}

	w.scheduleFlush(ctx, acc.DedupKey, time.Until(acc.FlushAfter))
	return nil
}

// Flush dispatches the currently pending items for a dedup key if this worker
// owns the lease and the bucket is due.
func (w *Worker) Flush(ctx context.Context, dedupKey string) error {
	now := w.now()
	locked, err := w.store.LockDueForFlush(ctx, dedupKey, w.ownerID, now)
	if err != nil || !locked {
		return err
	}

	items, itemIDs, err := w.store.PendingItems(ctx, dedupKey)
	if err != nil {
		_ = w.store.ReleaseLease(ctx, dedupKey, w.ownerID)
		return err
	}
	if len(items) == 0 {
		return w.store.MarkFlushed(ctx, dedupKey, nil)
	}

	appctx.GetLogger(ctx).Info().
		Str("worker", w.ownerID).
		Str("dedup_key", dedupKey).
		Int("item_count", len(items)).
		Msg("notifications: flushing accumulator")

	envelope := w.accumulate(items)
	if err := w.dispatcher.Dispatch(ctx, envelope); err != nil {
		_ = w.store.ReleaseLease(ctx, dedupKey, w.ownerID)
		return err
	}

	return w.store.MarkFlushed(ctx, dedupKey, itemIDs)
}

func (w *Worker) accumulate(items []model.Envelope) model.Envelope {
	envelope := items[0]
	moreCount := max(len(items)-w.maxRenderedItems, 0)
	renderedItems := make([]map[string]any, 0, min(len(items), w.maxRenderedItems))

	for _, item := range items[:min(len(items), w.maxRenderedItems)] {
		renderedItems = append(renderedItems, item.TemplateData)
	}

	envelope.TemplateData = map[string]any{
		"_count":     len(items),
		"_items":     renderedItems,
		"_moreCount": moreCount,
	}
	return envelope
}

func (w *Worker) scheduleFlush(ctx context.Context, dedupKey string, delay time.Duration) {
	if delay < 0 {
		delay = 0
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if timer, ok := w.timers[dedupKey]; ok {
		timer.Stop()
	}

	w.timers[dedupKey] = time.AfterFunc(delay, func() {
		_ = w.Flush(ctx, dedupKey)
	})
}

func handlerNames(rules map[string]model.HandlerRule) []string {
	names := make([]string, 0, len(rules))
	for name := range rules {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func renderDedupKey(templateString string, envelope model.Envelope) (string, error) {
	if templateString == "" {
		return "", errors.New("accumulated notification rule requires a dedup key template")
	}

	tmpl, err := template.New("dedup_key").Option("missingkey=error").Parse(templateString)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, envelope); err != nil {
		return "", err
	}

	key := strings.TrimSpace(buf.String())
	if key == "" {
		return "", errors.New("accumulated notification rule rendered an empty dedup key")
	}
	return key, nil
}

func perRecipientAccumulationEnvelopes(envelope model.Envelope) ([]model.Envelope, error) {
	envelopes := make([]model.Envelope, 0, len(envelope.Recipients))
	for _, recipient := range envelope.Recipients {
		recipient = strings.TrimSpace(recipient)
		if recipient == "" {
			continue
		}

		recipientEnvelope := envelope
		recipientEnvelope.ID = perRecipientItemID(envelope.ID, recipient)
		recipientEnvelope.Recipients = []string{recipient}
		recipientEnvelope.DedupKey = perRecipientDedupKey(recipient, envelope.DedupKey)
		envelopes = append(envelopes, recipientEnvelope)
	}
	if len(envelopes) == 0 {
		return nil, errors.New("accumulated notification requires at least one non-empty recipient")
	}
	return envelopes, nil
}

func perRecipientDedupKey(recipient, dedupKey string) string {
	return recipient + ":" + dedupKey
}

func perRecipientItemID(id, recipient string) string {
	return fmt.Sprintf("%s:%x", id, fnvHash(recipient))
}

func fnvHash(value string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(value))
	return h.Sum64()
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}

	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
