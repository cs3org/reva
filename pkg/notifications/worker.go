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

	"github.com/cs3org/reva/v3/pkg/notifications/handlers"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
)

const defaultMaxRenderedItems = 10

// AccumulationStore persists accumulated notifications and coordinates leases.
type AccumulationStore interface {
	Add(ctx context.Context, envelope model.Envelope, now time.Time) (*model.Bucket, error)
	AcquireLease(ctx context.Context, dedupKey, owner string, leaseUntil, now time.Time) (bool, error)
	LockDueForFlush(ctx context.Context, dedupKey, owner string, now time.Time) (bool, error)
	PendingItems(ctx context.Context, dedupKey string) ([]model.Envelope, []string, error)
	MarkFlushed(ctx context.Context, dedupKey string, itemIDs []string) error
	ReleaseLease(ctx context.Context, dedupKey, owner string) error
	ListCandidates(ctx context.Context, now time.Time, limit int) ([]*model.Bucket, error)
}

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
	store      AccumulationStore
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
func NewWorker(store AccumulationStore, dispatcher *handlers.Dispatcher, conf WorkerConfig) (*Worker, error) {
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
		rules:            cloneEventRules(conf.EventRules),
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

		for _, recipientEnvelope := range recipientEnvelopes {
			bucket, err := w.store.Add(ctx, recipientEnvelope, w.now())
			if err != nil {
				return err
			}
			if err := w.resumeBucket(ctx, bucket); err != nil {
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

func (w *Worker) resumeBucket(ctx context.Context, bucket *model.Bucket) error {
	now := w.now()
	leaseUntil := bucket.FlushAfter.Add(w.leaseDuration)
	if leaseUntil.Before(now.Add(w.leaseDuration)) {
		leaseUntil = now.Add(w.leaseDuration)
	}

	acquired, err := w.store.AcquireLease(ctx, bucket.DedupKey, w.ownerID, leaseUntil, now)
	if err != nil || !acquired {
		return err
	}

	if bucket.ItemCount >= bucket.MaxItems || !bucket.FlushAfter.After(now) {
		return w.Flush(ctx, bucket.DedupKey)
	}

	w.scheduleFlush(ctx, bucket.DedupKey, time.Until(bucket.FlushAfter))
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

func cloneEventRules(in map[string]model.EventRule) map[string]model.EventRule {
	out := make(map[string]model.EventRule, len(in))
	for eventType, rule := range in {
		rule.Handlers = cloneHandlerRules(rule.Handlers)
		out[eventType] = rule
	}
	return out
}

func cloneHandlerRules(in map[string]model.HandlerRule) map[string]model.HandlerRule {
	out := make(map[string]model.HandlerRule, len(in))
	for name, rule := range in {
		out[name] = rule
	}
	return out
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
	return fmt.Sprintf("%d:%s:%s", len(recipient), recipient, dedupKey)
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
