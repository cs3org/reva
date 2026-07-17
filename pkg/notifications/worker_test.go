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
	"testing"

	"github.com/cs3org/reva/v3/pkg/notifications/handlers"
	"github.com/cs3org/reva/v3/pkg/notifications/model"
)

type recordingHandler struct {
	name      string
	envelopes []model.Envelope
}

func (h *recordingHandler) Name() string {
	return h.name
}

func (h *recordingHandler) Send(_ context.Context, envelope model.Envelope) error {
	h.envelopes = append(h.envelopes, envelope)
	return nil
}

func TestWorkerResolvesDirectEventRule(t *testing.T) {
	ctx := context.Background()
	handler := &recordingHandler{name: handlers.EmailHandlerName}
	worker, err := NewWorker(nil, handlers.NewDispatcher(handler), WorkerConfig{
		OwnerID: "box-1",
		EventRules: map[string]model.EventRule{
			"office.mention": {
				Type: model.TypeDirect,
				Handlers: map[string]model.HandlerRule{
					handlers.EmailHandlerName: {TemplateName: "office-mention"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	err = worker.Handle(ctx, model.Envelope{
		ID:        "not-1",
		EventType: "office.mention",
		Recipients: []string{
			"bob@example.org",
		},
		TemplateData: map[string]any{
			"document_id": "doc-1",
		},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if len(handler.envelopes) != 1 {
		t.Fatalf("dispatched envelopes = %d, want 1", len(handler.envelopes))
	}
	envelope := handler.envelopes[0]
	if envelope.Type != model.TypeDirect {
		t.Fatalf("delivery type = %q, want direct", envelope.Type)
	}
	if envelope.TemplateName != "office-mention" {
		t.Fatalf("template name = %q, want office-mention", envelope.TemplateName)
	}
	if envelope.TemplateData["document_id"] != "doc-1" {
		t.Fatalf("template data = %+v, want submitted template data", envelope.TemplateData)
	}
}

func TestWorkerRejectsUnknownEventType(t *testing.T) {
	worker, err := NewWorker(nil, handlers.NewDispatcher(), WorkerConfig{
		OwnerID: "box-1",
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	if err := worker.Handle(context.Background(), model.Envelope{EventType: "missing"}); err == nil {
		t.Fatal("expected unknown event type to fail")
	}
}

func TestWorkerAccumulatesUsingRuleDedupKey(t *testing.T) {
	ctx := context.Background()
	store := newTestGORMStore(t)
	handler := &recordingHandler{name: handlers.EmailHandlerName}
	worker, err := NewWorker(store, handlers.NewDispatcher(handler), WorkerConfig{
		OwnerID: "box-1",
		EventRules: map[string]model.EventRule{
			"share.created": {
				Type:             model.TypeAccumulated,
				DedupKeyTemplate: "{{ .TemplateData.share_id }}",
				Handlers: map[string]model.HandlerRule{
					handlers.EmailHandlerName: {TemplateName: "share-created"},
				},
				Accumulation: model.AccumulationPolicy{
					WindowSeconds: 3600,
					MaxItems:      1,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	err = worker.Handle(ctx, model.Envelope{
		ID:        "not-1",
		EventType: "share.created",
		Recipients: []string{
			"bob@example.org",
		},
		TemplateData: map[string]any{
			"share_id": "share-1",
			"name":     "report.pdf",
		},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if len(handler.envelopes) != 1 {
		t.Fatalf("dispatched envelopes = %d, want 1", len(handler.envelopes))
	}
	envelope := handler.envelopes[0]
	if envelope.Type != model.TypeAccumulated {
		t.Fatalf("delivery type = %q, want accumulated", envelope.Type)
	}
	if envelope.DedupKey != perRecipientDedupKey("bob@example.org", "share-1") {
		t.Fatalf("dedup key = %q, want recipient-scoped share key", envelope.DedupKey)
	}
	if len(envelope.Recipients) != 1 || envelope.Recipients[0] != "bob@example.org" {
		t.Fatalf("recipients = %+v, want only bob@example.org", envelope.Recipients)
	}
	if envelope.TemplateData["_count"] != 1 {
		t.Fatalf("template data = %+v, want accumulated count", envelope.TemplateData)
	}
}

func TestWorkerAccumulatesMultiRecipientEventsSeparately(t *testing.T) {
	ctx := context.Background()
	store := newTestGORMStore(t)
	handler := &recordingHandler{name: handlers.EmailHandlerName}
	worker, err := NewWorker(store, handlers.NewDispatcher(handler), WorkerConfig{
		OwnerID: "box-1",
		EventRules: map[string]model.EventRule{
			"EmailReminder": {
				Type:             model.TypeAccumulated,
				DedupKeyTemplate: "{{ .TemplateData.share_id }}",
				Handlers: map[string]model.HandlerRule{
					handlers.EmailHandlerName: {TemplateName: "email-reminder"},
				},
				Accumulation: model.AccumulationPolicy{
					WindowSeconds: 3600,
					MaxItems:      1,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	err = worker.Handle(ctx, model.Envelope{
		ID:        "not-1",
		EventType: "EmailReminder",
		Recipients: []string{
			"bob@example.org",
			"carol@example.org",
		},
		TemplateData: map[string]any{
			"share_id": "share-1",
		},
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if len(handler.envelopes) != 2 {
		t.Fatalf("dispatched envelopes = %d, want 2", len(handler.envelopes))
	}

	seenDedupKeys := map[string]bool{}
	for _, envelope := range handler.envelopes {
		if len(envelope.Recipients) != 1 {
			t.Fatalf("recipients = %+v, want per-recipient envelope", envelope.Recipients)
		}
		recipient := envelope.Recipients[0]
		wantDedupKey := perRecipientDedupKey(recipient, "share-1")
		if envelope.DedupKey != wantDedupKey {
			t.Fatalf("dedup key = %q, want %q", envelope.DedupKey, wantDedupKey)
		}
		if seenDedupKeys[envelope.DedupKey] {
			t.Fatalf("dedup key %q was reused across recipients", envelope.DedupKey)
		}
		seenDedupKeys[envelope.DedupKey] = true
	}
}
