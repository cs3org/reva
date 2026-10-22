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

package ocmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

const testSecret = "hfiuhworzwnur98d3wjiwhr"

type envelopeCase struct {
	ID           string          `json:"id"`
	Purpose      string          `json:"purpose"`
	Body         json.RawMessage `json:"body"`
	HTTP         int             `json:"http"`
	HandlerCalls int             `json:"handlerCalls"`
}

// deferredEnvelopes are rows whose final expectation needs a type registered
// later in the stack; until then the type is unknown and ignored.
var deferredEnvelopes = map[string]envelopeCase{
	"ENV-039": {HTTP: 201},
	"ENV-040": {HTTP: 201},
}

func TestParseNotificationEnvelopeCorpus(t *testing.T) {
	raw, err := os.ReadFile("testdata/notification-envelopes.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []envelopeCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("empty corpus")
	}
	for _, tc := range cases {
		t.Run(tc.ID, func(t *testing.T) {
			want := tc
			if d, ok := deferredEnvelopes[tc.ID]; ok {
				want.HTTP, want.HandlerCalls = d.HTTP, d.HandlerCalls
			}
			reg := NewNotificationRegistry()
			h := &recordingHandler{}
			for typ := range builtinContracts {
				if !IsRequestNotification(typ) {
					if err := reg.SetHandler(typ, h); err != nil {
						t.Fatal(err)
					}
				}
			}
			n, err := reg.Parse(tc.Body)
			if got := int(ReceiverStatus(err)); got != want.HTTP {
				t.Fatalf("%s: status %d, want %d (%v)", tc.Purpose, got, want.HTTP, err)
			}
			if err == nil {
				if _, err := reg.Dispatch(context.Background(), n); err != nil {
					t.Fatal(err)
				}
			}
			if len(h.seen) != want.HandlerCalls {
				t.Fatalf("%s: handler calls %d, want %d", tc.Purpose, len(h.seen), want.HandlerCalls)
			}
		})
	}
}

func TestParseNotificationFields(t *testing.T) {
	reg := NewNotificationRegistry()
	n := mustParse(t, reg, `{"notificationType":"SHARE_CHANGE_PERMISSION","senderDomain":"a.example.org","resourceType":"folder","shareType":"user","notification":{"message":"m","sharedSecret":"`+testSecret+`","file":{"providerId":"42","permissions":["read","write"]}}}`)
	if n.Class != NotificationClassSpec || n.ProviderID != "42" || n.ResourceType != "folder" || n.ShareType != "user" ||
		n.File == nil || strings.Join(n.File.Permissions, ",") != "read,write" || n.SharedSecret.Reveal() != testSecret {
		t.Fatalf("fields %#v %#v", n, n.File)
	}
	n = mustParse(t, reg, `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"Alice @x@a.example.org"}}}`)
	if n.User == nil || n.User.UserID != "Alice @x@a.example.org" {
		t.Fatalf("user %#v", n.User)
	}
	// senderDomain is the complete host and port, lowercased only.
	n = mustParse(t, reg, `{"notificationType":"ACME","senderDomain":"A.Example.org:0443","shareType":"user","providerId":"flat"}`)
	if n.Class != NotificationClassUnknown || n.SenderDomain != "a.example.org:0443" || n.ProviderID != "" {
		t.Fatalf("unknown %#v", n)
	}
	n = mustParse(t, reg, `{"notificationType":"\u0085EXT\u2003","senderDomain":"[2001:DB8::1]:8443","shareType":"user"}`)
	if n.Type != "\u0085EXT\u2003" || n.SenderDomain != "[2001:db8::1]:8443" {
		t.Fatalf("identity %q %q", n.Type, n.SenderDomain)
	}
}

func TestParseNotificationRejectsFlatProviderFallback(t *testing.T) {
	reg := NewNotificationRegistry()
	for _, body := range []string{
		`{"notificationType":"SHARE_ACCEPTED","senderDomain":"b.example.org","resourceType":"file","providerId":"42","notification":{"file":{}}}`,
		`{"notificationType":"SHARE_ACCEPTED","senderDomain":"b.example.org","resourceType":"file","providerId":"42","notification":{"file":{"providerId":null}}}`,
		`{"notificationType":"SHARE_ACCEPTED","senderDomain":"b.example.org","resourceType":"file","providerId":"42"}`,
		`{"notificationType":"SHARE_ACCEPTED","resourceType":"file","notification":{"file":{"providerId":"42"}}}`,
		`{"notificationType":"SHARE_CHANGE_PERMISSION","senderDomain":"b.example.org","resourceType":"file","notification":{"file":{"providerId":"42","permissions":null}}}`,
	} {
		if _, err := reg.Parse([]byte(body)); ReceiverStatus(err) != NotificationStatusBadRequest {
			t.Fatalf("%s: %v", body, err)
		}
	}
}

// A present selector must be a non-empty string even when the other one is valid.
func TestParseNotificationRejectsNullSelectorBesideValidOne(t *testing.T) {
	reg := NewNotificationRegistry()
	for _, body := range []string{
		`{"notificationType":"ACME","senderDomain":"a.example.org","resourceType":null,"shareType":"user"}`,
		`{"notificationType":"ACME","senderDomain":"a.example.org","resourceType":"file","shareType":""}`,
		`{"notificationType":"ACME","senderDomain":"a.example.org","resourceType":"file","shareType":null}`,
	} {
		if _, err := reg.Parse([]byte(body)); ReceiverStatus(err) != NotificationStatusBadRequest {
			t.Fatalf("%s: %v", body, err)
		}
	}
}

func TestNotificationErrorsCarryNoValues(t *testing.T) {
	reg := NewNotificationRegistry()
	bodies := []string{
		`{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"leaky-value"}}}`,
		`{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"sharedSecret":["leaky-value"],"file":{"providerId":"42"}}}`,
		`{"notificationType":"leaky-value"`,
	}
	for _, b := range bodies {
		_, err := reg.Parse([]byte(b))
		if err == nil || strings.Contains(err.Error(), "leaky-value") {
			t.Fatalf("error %v", err)
		}
	}
}

func TestNotificationSecretIsRedacted(t *testing.T) {
	reg := NewNotificationRegistry()
	n, err := reg.Parse([]byte(`{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"sharedSecret":"` + testSecret + `","file":{"providerId":"42"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := reg.Parse([]byte(`{"notificationType":"ACME","senderDomain":"a.example.org","shareType":"user","notification":{"sharedSecret":"` + testSecret + `"}}`))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	log := zerolog.New(&buf)
	for _, v := range []*Notification{n, unknown} {
		raw, _ := json.Marshal(v)
		buf.WriteString(string(raw))
		buf.WriteString(fmt.Sprintf("%v %+v %#v %s", v, *v, v, v))
		buf.WriteString(fmt.Sprintf("%v %+v %#v", v.SharedSecret, v.SharedSecret, v.SharedSecret))
		log.Info().Interface("n", v).Str("secret", v.SharedSecret.String()).Msg("logged")
		log.Info().Msgf("%+v", v)
	}
	if strings.Contains(buf.String(), testSecret) {
		t.Fatalf("secret leaked: %s", buf.String())
	}
	if n.SharedSecret.Reveal() != testSecret {
		t.Fatal("secret not available for comparison")
	}
}

func TestNotificationCorePortability(t *testing.T) {
	allowed := map[string]bool{
		"bytes": true, "context": true, "encoding/json": true, "errors": true,
		"fmt": true, "slices": true, "strconv": true, "strings": true, "sync": true, "time": true,
		"github.com/cs3org/reva/v3/pkg/ocm/providerdomain": true,
	}
	for _, f := range []string{"specs_notifications.go", "notification_handlers.go", "notification_sender_state.go"} {
		file, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range file.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if !allowed[path] {
				t.Errorf("%s imports %s", f, path)
			}
		}
	}
}
