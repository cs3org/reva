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
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

type envelopeCase struct {
	ID           string            `json:"id"`
	Purpose      string            `json:"purpose"`
	Body         json.RawMessage   `json:"body"`
	HandlerCalls int               `json:"handlerCalls"`
	Class        NotificationClass `json:"class"`
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
			reg := NewNotificationRegistry()
			h := &recordingHandler{}
			for typ := range builtinContracts {
				if err := reg.SetHandler(typ, h); err != nil {
					t.Fatal(err)
				}
			}
			n, err := reg.Parse(tc.Body)
			if n.Class != tc.Class {
				t.Fatalf("%s: class %s, want %s", tc.Purpose, n.Class, tc.Class)
			}
			code, contractError := NotificationErrorCodeOf(err)
			switch tc.Class {
			case NotificationClassSpec, NotificationClassUnknown:
				if err != nil {
					t.Fatalf("%s: unexpected parse error: %v", tc.Purpose, err)
				}
			case NotificationClassMalformed:
				if err == nil || !contractError || code == NotificationErrNotImplemented {
					t.Fatalf("%s: expected malformed contract error, got %v", tc.Purpose, err)
				}
			case NotificationClassRequest:
				if !contractError || code != NotificationErrNotImplemented {
					t.Fatalf("%s: expected not-implemented error, got %v", tc.Purpose, err)
				}
			default:
				t.Fatalf("%s: invalid expected class %q", tc.Purpose, tc.Class)
			}
			out, dispatchErr := reg.Dispatch(context.Background(), n)
			if dispatchErr != nil {
				t.Fatal(dispatchErr)
			}
			wantOutcome := DispatchIgnored
			if tc.HandlerCalls > 0 {
				wantOutcome = DispatchHandled
			}
			if tc.Class == NotificationClassRequest {
				wantOutcome = DispatchNotImplemented
			}
			if out != wantOutcome || len(h.seen) != tc.HandlerCalls {
				t.Fatalf("%s: outcome %s calls %d, want %s %d", tc.Purpose, out, len(h.seen), wantOutcome, tc.HandlerCalls)
			}
		})
	}
}

func TestParseNotificationFields(t *testing.T) {
	reg := NewNotificationRegistry()
	n := mustParse(t, reg, `{"notificationType":"SHARE_CHANGE_PERMISSION","senderDomain":"a.example.org","resourceType":"folder","shareType":"user","notification":{"message":"m","file":{"providerId":"42","permissions":["read","write"]}}}`)
	wantFile := &NotificationFile{ProviderID: "42", Permissions: []string{"read", "write"}}
	if n.Class != NotificationClassSpec || n.ResourceType != "folder" || n.ShareType != "user" || n.Message != "m" || !reflect.DeepEqual(n.File, wantFile) {
		t.Fatalf("fields %#v file %#v", n, n.File)
	}
	n = mustParse(t, reg, `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"Alice @x@a.example.org"}}}`)
	if n.User == nil || *n.User != (NotificationUser{UserID: "Alice @x@a.example.org"}) {
		t.Fatalf("user %#v", n.User)
	}
	n = mustParse(t, reg, `{"notificationType":"CONTACT_REMOVED","senderDomain":"A.Example.org:0443","shareType":"user","notification":{"user":{"userId":"Alice @x@A.Example.org:0443","targetUserId":"Bob Smith@B.Example.org:8443"}}}`)
	if n.SenderDomain != "a.example.org:0443" || n.User == nil || *n.User != (NotificationUser{UserID: "Alice @x@A.Example.org:0443", TargetUserID: "Bob Smith@B.Example.org:8443"}) {
		t.Fatalf("contact %#v %#v", n, n.User)
	}
	n = mustParse(t, reg, `{"notificationType":"GROUP_REMOVED","senderDomain":"a.example.org","shareType":"group","notification":{"group":{"groupId":"group @ opaque"}}}`)
	if n.Group == nil || n.Group.GroupID != "group @ opaque" {
		t.Fatalf("group %#v", n.Group)
	}
	n = mustParse(t, reg, `{"notificationType":"ACME","senderDomain":"A.Example.org:0443","shareType":"user","providerId":"flat"}`)
	if n.Class != NotificationClassUnknown || n.SenderDomain != "a.example.org:0443" || n.File != nil {
		t.Fatalf("unknown %#v", n)
	}
	n = mustParse(t, reg, `{"notificationType":"\u0085EXT\u2003","senderDomain":"[2001:DB8::1]:8443","shareType":"user"}`)
	if n.Type != "\u0085EXT\u2003" || n.SenderDomain != "[2001:db8::1]:8443" {
		t.Fatalf("identity %q %q", n.Type, n.SenderDomain)
	}
	for _, tc := range []struct {
		body string
		file NotificationFile
	}{
		{`{"notificationType":"REQUEST_RESHARE","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"grant @ opaque","shareWith":"Bob @B.Example.org:8443"}}}`, NotificationFile{ProviderID: "grant @ opaque", ShareWith: "Bob @B.Example.org:8443"}},
	} {
		n, err := reg.Parse([]byte(tc.body))
		if code, ok := NotificationErrorCodeOf(err); !ok || code != NotificationErrNotImplemented || !reflect.DeepEqual(n.File, &tc.file) {
			t.Fatalf("request file %#v: %v", n.File, err)
		}
	}
}

func TestParseNotificationPermissionPresence(t *testing.T) {
	reg := NewNotificationRegistry()
	const head = `{"notificationType":"SHARE_CHANGE_PERMISSION","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"42"`
	n := mustParse(t, reg, head+`}}}`)
	if n.File.Permissions != nil {
		t.Fatalf("absent permissions became %#v", n.File.Permissions)
	}
	n = mustParse(t, reg, head+`,"permissions":[]}}}`)
	if n.File.Permissions == nil || len(n.File.Permissions) != 0 {
		t.Fatalf("empty permissions became %#v", n.File.Permissions)
	}
	all := []string{"read", "create", "update", "delete", "write", "share", "view"}
	n = mustParse(t, reg, head+`,"permissions":["read","create","update","delete","write","share","view"]}}}`)
	if !reflect.DeepEqual(n.File.Permissions, all) {
		t.Fatalf("permissions %#v", n.File.Permissions)
	}
}

func TestParseCollectionDeclineIsNotAFileGrant(t *testing.T) {
	reg := NewNotificationRegistry()
	h := &recordingHandler{}
	if err := reg.SetHandler(NotificationShareDeclined, h); err != nil {
		t.Fatal(err)
	}
	n, err := reg.Parse([]byte(`{"notificationType":"SHARE_DECLINED","senderDomain":"b.example.org","resourceType":"collection","notification":{"file":{"providerId":"grant-1"}}}`))
	if code, ok := NotificationErrorCodeOf(err); !ok || code != NotificationErrNotImplemented || n.Class != NotificationClassRequest {
		t.Fatalf("class %s: %v", n.Class, err)
	}
	if n.File != nil {
		t.Fatalf("collection decline exposed a file grant: %#v", n)
	}
	if out, err := reg.Dispatch(context.Background(), n); out != DispatchNotImplemented || err != nil || len(h.seen) != 0 {
		t.Fatalf("dispatch %s, error %v, handler calls %d", out, err, len(h.seen))
	}
}

func TestRegistryRejectsInvalidNotificationFields(t *testing.T) {
	const grant = `{"notificationType":"SHARE_CHANGE_PERMISSION","senderDomain":"a.example.org","resourceType":"folder","notification":{"file":{"providerId":"%s"}}}`
	const contact = `{"notificationType":"CONTACT_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org","targetUserId":"%s@b.example.org"}}}`
	const server = `{"notificationType":"OCM_SERVER_ADDED","senderDomain":"a.example.org","resourceType":"federation","notification":{"federation":{"federationId":"mesh","url":"%s"%s}}}`

	for _, tc := range []struct {
		name, body, field string
	}{
		{"relative federation URL", fmt.Sprintf(server, "/ocm", ""), "notification.federation.url"},
		{"hostless federation URL", fmt.Sprintf(server, "https://:8443", ""), "notification.federation.url"},
		{"federation URL scheme", fmt.Sprintf(server, "ftp://b.example.org", ""), "notification.federation.url"},
		{"federation URL path", fmt.Sprintf(server, "https://b.example.org/ocm", ""), "notification.federation.url"},
		{"federation URL userinfo", fmt.Sprintf(server, "https://user@b.example.org", ""), "notification.federation.url"},
		{"federation URL query", fmt.Sprintf(server, "https://b.example.org?", ""), "notification.federation.url"},
		{"federation URL fragment", fmt.Sprintf(server, "https://b.example.org#", ""), "notification.federation.url"},
		{"numeric federation name", fmt.Sprintf(server, "https://b.example.org", `,"name":7`), "notification.federation.name"},
		{"null federation name", fmt.Sprintf(server, "https://b.example.org", `,"name":null`), "notification.federation.name"},
		{"boolean federation group", fmt.Sprintf(server, "https://b.example.org", `,"mlsGroupId":true`), "notification.federation.mlsGroupId"},
		{"null federation group", fmt.Sprintf(server, "https://b.example.org", `,"mlsGroupId":null`), "notification.federation.mlsGroupId"},
		{"unpaired high surrogate in grant", fmt.Sprintf(grant, `\ud800`), "notification.file.providerId"},
		{"unpaired low surrogate in address", fmt.Sprintf(contact, `\udc00`), "notification.user.targetUserId"},
		{"interrupted pair in address", fmt.Sprintf(contact, `\ud800x\udc00`), "notification.user.targetUserId"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg := NewNotificationRegistry()
			h := &recordingHandler{}
			for typ := range builtinContracts {
				if err := reg.SetHandler(typ, h); err != nil {
					t.Fatal(err)
				}
			}
			n, parseErr := reg.Parse([]byte(tc.body))
			outcome, dispatchErr := reg.Dispatch(context.Background(), n)
			if n.Class != NotificationClassMalformed || NotificationReason(parseErr) != "invalid "+tc.field {
				t.Errorf("class %s, error %v, want malformed and invalid %s", n.Class, parseErr, tc.field)
			}
			if outcome != DispatchIgnored || dispatchErr != nil || len(h.seen) != 0 {
				t.Errorf("dispatch %s, error %v, handler calls %d", outcome, dispatchErr, len(h.seen))
			}
		})
	}
}

func TestNotificationEnvelopeTokensAreLossless(t *testing.T) {
	const unshare = `{"notificationType":"%s","senderDomain":"a.example.org","resourceType":"%s","notification":{"file":{"providerId":"42"}}}`
	const custom = `{"notificationType":"ACME","senderDomain":"a.example.org","shareType":"%s"}`
	for _, tc := range []struct {
		name, body, reason string
		class              NotificationClass
		calls              int
	}{
		{"replacement in type", fmt.Sprintf(unshare, `ACME\ufffd`, "file"), "", NotificationClassUnknown, 0},
		{"surrogate in unknown type", fmt.Sprintf(unshare, `ACME\ud800`, "file"), "malformed notification", NotificationClassMalformed, 0},
		{"surrogate in known type", fmt.Sprintf(unshare, `SHARE_UNSHARED\udc00`, "file"), "malformed notification", NotificationClassMalformed, 0},
		{"replacement in resourceType", fmt.Sprintf(unshare, "SHARE_UNSHARED", `file\ufffd`), "", NotificationClassSpec, 1},
		{"surrogate in resourceType", fmt.Sprintf(unshare, "SHARE_UNSHARED", `file\ud800`), "invalid resourceType", NotificationClassMalformed, 0},
		{"replacement in shareType", fmt.Sprintf(custom, `user\ufffd`), "", NotificationClassUnknown, 0},
		{"surrogate in shareType", fmt.Sprintf(custom, `user\udc00`), "invalid shareType", NotificationClassMalformed, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg := NewNotificationRegistry()
			h := &recordingHandler{}
			for typ := range builtinContracts {
				if err := reg.SetHandler(typ, h); err != nil {
					t.Fatal(err)
				}
			}
			n, err := reg.Parse([]byte(tc.body))
			if n.Class != tc.class || (tc.reason == "") != (err == nil) || err != nil && NotificationReason(err) != tc.reason {
				t.Fatalf("class %s, error %v; want %s %q", n.Class, err, tc.class, tc.reason)
			}
			if err == nil && !strings.ContainsRune(n.Type+n.ResourceType+n.ShareType, '\uFFFD') {
				t.Fatalf("replacement character was not preserved: %q %q %q", n.Type, n.ResourceType, n.ShareType)
			}
			if _, err := reg.Dispatch(context.Background(), n); err != nil || len(h.seen) != tc.calls {
				t.Fatalf("dispatch error %v, handler calls %d, want %d", err, len(h.seen), tc.calls)
			}
		})
	}
}

func TestNotificationErrorsCarryNoValues(t *testing.T) {
	const key = "caller-chosen-member-name"
	reg := NewNotificationRegistry()
	h := &recordingHandler{}
	if err := reg.SetHandler(NotificationShareUnshared, h); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ body, reason string }{
		{`{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"leaky-value"}}}`, "invalid notification.user.userId"},
		{`{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"sharedSecret":["leaky-value"],"file":{"providerId":"42"}}}`, "invalid notification"},
		{`{"notificationType":"leaky-value"`, "malformed notification"},
		{`{"notificationType":"ACME","senderDomain":"a.example.org","shareType":"user","notification":{"` + key + `":"leaky-value"}}`, "invalid notification"},
		{`{"notificationType":"ACME","senderDomain":"a.example.org","shareType":"user","notification":{"` + key + `":null}}`, "invalid notification"},
		{`{"notificationType":"ACME","senderDomain":"a.example.org","shareType":"user","notification":{"` + key + `":null,"message":7}}`, "invalid notification.message"},
		{`{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"` + key + `":["leaky-value"],"file":{"providerId":"42"}}}`, "invalid notification"},
	}
	for _, tc := range cases {
		n, err := reg.Parse([]byte(tc.body))
		if err == nil || NotificationReason(err) != tc.reason || err.Error() != tc.reason {
			t.Fatalf("reason %q, want %q (%v)", NotificationReason(err), tc.reason, err)
		}
		var ne *NotificationError
		if strings.Contains(tc.body, key) && (!errors.As(err, &ne) || ne.Code != NotificationErrInvalidField || strings.Contains(ne.Field, key)) {
			t.Fatalf("error %#v", ne)
		}
		if out, err := reg.Dispatch(context.Background(), n); err != nil || out != DispatchIgnored {
			t.Fatalf("invalid dispatch %s %v", out, err)
		}
	}
	if len(h.seen) != 0 {
		t.Fatalf("handlers ran for invalid notifications: %v", h.seen)
	}
	for _, err := range []error{
		fmt.Errorf("leaky-value: %w", &NotificationError{Code: NotificationErrInvalidField, Field: "notification.leaky-value"}),
		errors.New("leaky-value"),
		&NotificationError{Code: NotificationErrorCode("leaky-value"), Field: "leaky-value"},
		&NotificationError{Code: NotificationErrInvalidField, Field: "notification.sharedSecret"},
	} {
		if got := NotificationReason(err); got != "invalid notification" {
			t.Fatalf("unsafe reason %q", got)
		}
	}
}

func TestNotificationPayloadFormatting(t *testing.T) {
	const canary = "opaque-payload-canary"
	reg := NewNotificationRegistry()
	for _, typ := range []string{"SHARE_UNSHARED", "EXAMPLE_CUSTOM_EVENT"} {
		n := mustParse(t, reg, `{"notificationType":"`+typ+`","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"42"},"sharedSecret":{"value":"`+canary+`"},"custom":{"nested":[null,true,"`+canary+`"]}}}`)
		if !bytes.Contains(n.Payload["sharedSecret"], []byte(canary)) || !bytes.Contains(n.Payload["custom"], []byte(canary)) {
			t.Fatal("opaque payload was altered")
		}
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "%v %+v %#v %s", n, *n, n, n)
		log := zerolog.New(&buf)
		log.Info().Msgf("%+v", n)
		if strings.Contains(buf.String(), canary) {
			t.Fatal("formatted notification exposed payload")
		}
	}
}

func TestNotificationRejectsInvalidUTF8(t *testing.T) {
	base := []byte(`{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"MARK@a.example.org"}}}`)
	reg := NewNotificationRegistry()
	for _, id := range [][]byte{{0xff}, {0xc0, 0xaf}, {0xe2, 0x82}} {
		body := bytes.Replace(base, []byte("MARK"), id, 1)
		n, err := reg.Parse(body)
		code, ok := NotificationErrorCodeOf(err)
		if !ok || code != NotificationErrMalformed || n.Class != NotificationClassMalformed {
			t.Fatalf("invalid UTF-8 %x admitted: %v", id, err)
		}
		if out, err := reg.Dispatch(context.Background(), n); out != DispatchIgnored || err != nil {
			t.Fatalf("invalid dispatch %s %v", out, err)
		}
	}
	for _, id := range []struct{ wire, want string }{
		{"\uFFFD", "\uFFFD"},
		{`\ufffd\uD83D\uDE00`, "\uFFFD\U0001f600"},
		{`\ufffd\\ud800`, "\uFFFD" + `\ud800`},
		{"\u00c9 @ User", "\u00c9 @ User"},
	} {
		n := mustParse(t, reg, string(bytes.Replace(base, []byte("MARK"), []byte(id.wire), 1)))
		if n.User == nil || n.User.UserID != id.want+"@a.example.org" {
			t.Fatalf("Unicode identity changed: %#v", n.User)
		}
		const grant = `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"%s"}}}`
		n = mustParse(t, reg, fmt.Sprintf(grant, id.wire))
		if n.File == nil || n.File.ProviderID != id.want {
			t.Fatalf("Unicode grant ID changed: %#v", n.File)
		}
	}
}
