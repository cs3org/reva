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
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

const testSecret = "hfiuhworzwnur98d3wjiwhr"

func TestParseNotificationMatrix(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		class    NotificationClass
		code     NotificationErrorCode
		field    string
		status   NotificationStatusClass
		provider string
		check    func(t *testing.T, n *Notification)
	}{
		{
			name:     "A share accepted nested",
			body:     `{"notificationType":"SHARE_ACCEPTED","senderDomain":"b.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`,
			class:    NotificationClassSpec,
			status:   NotificationStatusCreated,
			provider: "42",
		},
		{
			name:     "A both resourceType and shareType",
			body:     `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","shareType":"user","notification":{"file":{"providerId":"42"}}}`,
			class:    NotificationClassSpec,
			status:   NotificationStatusCreated,
			provider: "42",
		},
		{
			name:     "A change permission keeps permissions",
			body:     `{"notificationType":"SHARE_CHANGE_PERMISSION","senderDomain":"a.example.org","resourceType":"folder","notification":{"file":{"providerId":"42","permissions":["read","write"]}}}`,
			class:    NotificationClassSpec,
			status:   NotificationStatusCreated,
			provider: "42",
			check: func(t *testing.T, n *Notification) {
				if n.File == nil || strings.Join(n.File.Permissions, ",") != "read,write" {
					t.Fatalf("permissions %#v", n.File)
				}
			},
		},
		{
			name:     "A declined with optional string secret",
			body:     `{"notificationType":"SHARE_DECLINED","senderDomain":"b.example.org","resourceType":"file","notification":{"sharedSecret":"` + testSecret + `","file":{"providerId":"42"}}}`,
			class:    NotificationClassSpec,
			status:   NotificationStatusCreated,
			provider: "42",
			check: func(t *testing.T, n *Notification) {
				if n.SharedSecret.Reveal() != testSecret {
					t.Fatal("secret not preserved")
				}
				if _, ok := n.Payload["sharedSecret"]; ok {
					t.Fatal("secret left in raw payload")
				}
			},
		},
		{
			name:     "B Nextcloud flat shape",
			body:     `{"notificationType":"SHARE_ACCEPTED","resourceType":"file","providerId":"7c08","notification":{"sharedSecret":"` + testSecret + `","message":"accepted","shareWith":"bob","senderId":"alice","shareType":"user"}}`,
			class:    NotificationClassLegacy,
			status:   NotificationStatusCreated,
			provider: "7c08",
			check: func(t *testing.T, n *Notification) {
				if !n.Shape.MissingSenderDomain || !n.Shape.EnvelopeProviderID || n.Shape.Alias {
					t.Fatalf("shape %+v", n.Shape)
				}
				if n.File == nil || n.File.ProviderID != "7c08" || n.Message != "accepted" {
					t.Fatalf("normalized %#v %q", n.File, n.Message)
				}
				if n.SharedSecret.Reveal() != testSecret {
					t.Fatal("secret not preserved")
				}
			},
		},
		{
			name:     "B nested providerId wins over envelope",
			body:     `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","providerId":"envelope","notification":{"file":{"providerId":"nested"}}}`,
			class:    NotificationClassSpec,
			status:   NotificationStatusCreated,
			provider: "nested",
			check: func(t *testing.T, n *Notification) {
				if !n.Shape.ProviderIDConflict || n.Shape.EnvelopeProviderID {
					t.Fatalf("shape %+v", n.Shape)
				}
			},
		},
		{
			name:     "B missing senderDomain only",
			body:     `{"notificationType":"SHARE_UNSHARED","resourceType":"file","notification":{"file":{"providerId":"42"}}}`,
			class:    NotificationClassLegacy,
			status:   NotificationStatusCreated,
			provider: "42",
		},
		{
			name:     "B RESHARE_UNDO keeps its identity",
			body:     `{"notificationType":"RESHARE_UNDO","senderDomain":"b.example.org","resourceType":"file","notification":{"file":{"providerId":"42"}}}`,
			class:    NotificationClassLegacy,
			status:   NotificationStatusCreated,
			provider: "42",
			check: func(t *testing.T, n *Notification) {
				if n.Type != NotificationReshareUndo || n.ContractType != NotificationShareUnshared || !n.Shape.Alias {
					t.Fatalf("alias identity %q %q %+v", n.Type, n.ContractType, n.Shape)
				}
			},
		},
		{
			name:   "A user removed",
			body:   `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org"}}}`,
			class:  NotificationClassSpec,
			status: NotificationStatusCreated,
			check: func(t *testing.T, n *Notification) {
				if n.User == nil || n.User.UserID != "alice@a.example.org" {
					t.Fatalf("user %#v", n.User)
				}
			},
		},
		{
			name:   "A group removed",
			body:   `{"notificationType":"GROUP_REMOVED","senderDomain":"a.example.org","shareType":"group","notification":{"group":{"groupId":"g1"}}}`,
			class:  NotificationClassSpec,
			status: NotificationStatusCreated,
			check: func(t *testing.T, n *Notification) {
				if n.Group == nil || n.Group.GroupID != "g1" {
					t.Fatalf("group %#v", n.Group)
				}
			},
		},
		{
			name:   "A opaque identifiers and host forms",
			body:   `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"nomen%nescio*@example.org@[2001:db8::1]:8443"}}}`,
			class:  NotificationClassSpec,
			status: NotificationStatusCreated,
		},
		{
			name:   "C unknown type without known fields",
			body:   `{"notificationType":"ACME_EVENT","senderDomain":"a.example.org"}`,
			class:  NotificationClassUnknown,
			status: NotificationStatusCreated,
		},
		{
			name:   "C contact removal is unknown to the core",
			body:   `{"notificationType":"CONTACT_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org"}}}`,
			class:  NotificationClassUnknown,
			status: NotificationStatusCreated,
			check: func(t *testing.T, n *Notification) {
				if n.User != nil || n.ContractType != "" {
					t.Fatal("unknown type was interpreted")
				}
			},
		},
		{
			name:   "M MLS type is not normalized",
			body:   `{"notificationType":"MLS_COMMIT","senderDomain":"a.example.org","shareType":"federation","providerId":"x","notification":{"federation":{"mlsGroupId":"g","content":"AAAA"}}}`,
			class:  NotificationClassMLS,
			status: NotificationStatusCreated,
			check: func(t *testing.T, n *Notification) {
				if n.File != nil || n.Shape != (NotificationShape{}) || n.ContractType != "" {
					t.Fatalf("mls normalized: %#v %+v", n.File, n.Shape)
				}
				if _, ok := n.Payload["federation"]; !ok {
					t.Fatal("mls payload lost")
				}
			},
		},
		{
			name:   "R request share",
			body:   `{"notificationType":"REQUEST_SHARE","senderDomain":"b.example.org","resourceType":"file"}`,
			class:  NotificationClassRequest,
			code:   NotificationErrNotImplemented,
			status: NotificationStatusNotImplemented,
		},
		{
			name:   "R request reshare",
			body:   `{"notificationType":"REQUEST_RESHARE","senderDomain":"b.example.org","resourceType":"file","notification":{"file":{"providerId":"42","shareWith":"c@c.example.org"}}}`,
			class:  NotificationClassRequest,
			code:   NotificationErrNotImplemented,
			status: NotificationStatusNotImplemented,
		},
		{name: "D invalid JSON", body: `{"notificationType":`, class: NotificationClassMalformed, code: NotificationErrMalformed, status: NotificationStatusBadRequest},
		{name: "D array body", body: `["SHARE_UNSHARED"]`, class: NotificationClassMalformed, code: NotificationErrMalformed, status: NotificationStatusBadRequest},
		{name: "D null body", body: `null`, class: NotificationClassMalformed, code: NotificationErrMalformed, status: NotificationStatusBadRequest},
		{name: "D empty body", body: ``, class: NotificationClassMalformed, code: NotificationErrMalformed, status: NotificationStatusBadRequest},
		{name: "D missing type", body: `{"senderDomain":"a.example.org","resourceType":"file"}`, class: NotificationClassMalformed, code: NotificationErrMissingType, status: NotificationStatusBadRequest},
		{name: "D null type", body: `{"notificationType":null,"resourceType":"file"}`, class: NotificationClassMalformed, code: NotificationErrMissingType, status: NotificationStatusBadRequest},
		{name: "D empty type", body: `{"notificationType":" ","resourceType":"file"}`, class: NotificationClassMalformed, code: NotificationErrMissingType, status: NotificationStatusBadRequest},
		{name: "D non-string type", body: `{"notificationType":7,"resourceType":"file"}`, class: NotificationClassMalformed, code: NotificationErrMalformed, status: NotificationStatusBadRequest},
		{name: "D non-string senderDomain", body: `{"notificationType":"ACME","senderDomain":1}`, class: NotificationClassMalformed, code: NotificationErrMalformed, status: NotificationStatusBadRequest},
		{name: "D notification not an object", body: `{"notificationType":"ACME","shareType":"user","notification":"x"}`, class: NotificationClassMalformed, code: NotificationErrMalformed, status: NotificationStatusBadRequest},
		{
			name: "400 share state without providerId",
			body: `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{}}}`,
			code: NotificationErrMissingField, field: "notification.file.providerId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 share state without notification",
			body: `{"notificationType":"SHARE_ACCEPTED","senderDomain":"b.example.org","resourceType":"file"}`,
			code: NotificationErrMissingField, field: "notification.file.providerId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 share state without resourceType",
			body: `{"notificationType":"SHARE_ACCEPTED","senderDomain":"b.example.org","shareType":"user","notification":{"file":{"providerId":"42"}}}`,
			code: NotificationErrMissingField, field: "resourceType", status: NotificationStatusBadRequest,
		},
		{
			name: "400 file not an object",
			body: `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"file":"42"}}`,
			code: NotificationErrInvalidField, field: "notification.file", status: NotificationStatusBadRequest,
		},
		{
			name: "400 numeric providerId",
			body: `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":42}}}`,
			code: NotificationErrInvalidField, field: "notification.file", status: NotificationStatusBadRequest,
		},
		{
			name: "400 object sharedSecret",
			body: `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"sharedSecret":{"x":1},"file":{"providerId":"42"}}}`,
			code: NotificationErrInvalidField, field: "notification.sharedSecret", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed without userId",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{}}}`,
			code: NotificationErrMissingField, field: "notification.user.userId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed without user",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user"}`,
			code: NotificationErrMissingField, field: "notification.user.userId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed userId not an address",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice"}}}`,
			code: NotificationErrInvalidField, field: "notification.user.userId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed empty id part",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"@a.example.org"}}}`,
			code: NotificationErrInvalidField, field: "notification.user.userId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed empty provider part",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@"}}}`,
			code: NotificationErrInvalidField, field: "notification.user.userId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed with resourceType",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","resourceType":"file","notification":{"user":{"userId":"alice@a.example.org"}}}`,
			code: NotificationErrInvalidField, field: "resourceType", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed without shareType",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","notification":{"user":{"userId":"alice@a.example.org"}}}`,
			code: NotificationErrMissingField, field: "shareType", status: NotificationStatusBadRequest,
		},
		{
			name: "400 group removed without groupId",
			body: `{"notificationType":"GROUP_REMOVED","senderDomain":"a.example.org","shareType":"group","notification":{"group":{}}}`,
			code: NotificationErrMissingField, field: "notification.group.groupId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed with null resourceType",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","resourceType":null,"notification":{"user":{"userId":"alice@a.example.org"}}}`,
			code: NotificationErrInvalidField, field: "resourceType", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed with empty resourceType",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","resourceType":"","notification":{"user":{"userId":"alice@a.example.org"}}}`,
			code: NotificationErrInvalidField, field: "resourceType", status: NotificationStatusBadRequest,
		},
		{
			name: "400 group removed with null resourceType",
			body: `{"notificationType":"GROUP_REMOVED","senderDomain":"a.example.org","shareType":"group","resourceType":null,"notification":{"group":{"groupId":"g1"}}}`,
			code: NotificationErrInvalidField, field: "resourceType", status: NotificationStatusBadRequest,
		},
		{
			name: "400 null shareType",
			body: `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","shareType":null,"notification":{"file":{"providerId":"42"}}}`,
			code: NotificationErrInvalidField, field: "shareType", status: NotificationStatusBadRequest,
		},
		{
			name: "400 null senderDomain",
			body: `{"notificationType":"SHARE_UNSHARED","senderDomain":null,"resourceType":"file","notification":{"file":{"providerId":"42"}}}`,
			code: NotificationErrInvalidField, field: "senderDomain", status: NotificationStatusBadRequest,
		},
		{
			name: "400 numeric message",
			body: `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"message":7,"file":{"providerId":"42"}}}`,
			code: NotificationErrInvalidField, field: "notification.message", status: NotificationStatusBadRequest,
		},
		{
			name: "400 null message",
			body: `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"message":null,"file":{"providerId":"42"}}}`,
			code: NotificationErrInvalidField, field: "notification.message", status: NotificationStatusBadRequest,
		},
		{
			name: "400 null sharedSecret",
			body: `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"sharedSecret":null,"file":{"providerId":"42"}}}`,
			code: NotificationErrInvalidField, field: "notification.sharedSecret", status: NotificationStatusBadRequest,
		},
		{
			name: "400 null file",
			body: `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","providerId":"42","notification":{"file":null}}`,
			code: NotificationErrInvalidField, field: "notification.file", status: NotificationStatusBadRequest,
		},
		{
			name: "400 undefined permission",
			body: `{"notificationType":"SHARE_CHANGE_PERMISSION","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"42","permissions":["read","admin"]}}}`,
			code: NotificationErrInvalidField, field: "notification.file.permissions", status: NotificationStatusBadRequest,
		},
		{
			name: "400 numeric permission",
			body: `{"notificationType":"SHARE_CHANGE_PERMISSION","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"42","permissions":[1]}}}`,
			code: NotificationErrInvalidField, field: "notification.file", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed empty userId",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":""}}}`,
			code: NotificationErrMissingField, field: "notification.user.userId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 group removed empty groupId",
			body: `{"notificationType":"GROUP_REMOVED","senderDomain":"a.example.org","shareType":"group","notification":{"group":{"groupId":""}}}`,
			code: NotificationErrMissingField, field: "notification.group.groupId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 null user",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":null}}`,
			code: NotificationErrInvalidField, field: "notification.user", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed host a.example.org/path",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org/path"}}}`,
			code: NotificationErrInvalidField, field: "notification.user.userId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed host a.example .org",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example .org"}}}`,
			code: NotificationErrInvalidField, field: "notification.user.userId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed host https://a.example.org",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@https://a.example.org"}}}`,
			code: NotificationErrInvalidField, field: "notification.user.userId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed host a.example.org?q",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org?q"}}}`,
			code: NotificationErrInvalidField, field: "notification.user.userId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed host a.example.org#f",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org#f"}}}`,
			code: NotificationErrInvalidField, field: "notification.user.userId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed host a.example.org:",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org:"}}}`,
			code: NotificationErrInvalidField, field: "notification.user.userId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed host a.example.org:http",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org:http"}}}`,
			code: NotificationErrInvalidField, field: "notification.user.userId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed host [2001:db8::1",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@[2001:db8::1"}}}`,
			code: NotificationErrInvalidField, field: "notification.user.userId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed host []",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@[]"}}}`,
			code: NotificationErrInvalidField, field: "notification.user.userId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 user removed host a%zzexample.org",
			body: `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a%zzexample.org"}}}`,
			code: NotificationErrInvalidField, field: "notification.user.userId", status: NotificationStatusBadRequest,
		},
		{
			name: "400 alias still needs providerId",
			body: `{"notificationType":"RESHARE_UNDO","senderDomain":"b.example.org","resourceType":"file"}`,
			code: NotificationErrMissingField, field: "notification.file.providerId", status: NotificationStatusBadRequest,
		},
	}
	reg := NewNotificationRegistry()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, err := reg.Parse([]byte(tc.body))
			if got := ReceiverStatus(err); got != tc.status {
				t.Fatalf("status %d, want %d (err %v)", got, tc.status, err)
			}
			if tc.code == "" {
				if err != nil {
					t.Fatalf("unexpected error %v", err)
				}
				if n.Class != tc.class {
					t.Fatalf("class %s, want %s", n.Class, tc.class)
				}
			} else {
				code, ok := NotificationErrorCodeOf(err)
				if !ok || code != tc.code {
					t.Fatalf("error %v, want code %s", err, tc.code)
				}
				if tc.field != "" && err.(*NotificationError).Field != tc.field {
					t.Fatalf("field %q, want %q", err.(*NotificationError).Field, tc.field)
				}
				if tc.class != "" && tc.class != NotificationClassUnknown && n.Class != tc.class {
					t.Fatalf("class %s, want %s", n.Class, tc.class)
				}
			}
			if tc.provider != "" && n.ProviderID != tc.provider {
				t.Fatalf("providerId %q, want %q", n.ProviderID, tc.provider)
			}
			if tc.check != nil {
				tc.check(t, n)
			}
		})
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
	unknown, err := reg.Parse([]byte(`{"notificationType":"ACME","shareType":"user","notification":{"sharedSecret":"` + testSecret + `"}}`))
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
		"fmt": true, "net/http": true, "slices": true, "strconv": true, "strings": true, "sync": true, "time": true,
		"github.com/go-playground/validator/v10": true,
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
