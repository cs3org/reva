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
	Reason       string            `json:"reason"`
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
			reason := ""
			if err != nil {
				reason = NotificationReason(err)
			}
			if reason != tc.Reason {
				t.Fatalf("%s: reason %q, want %q", tc.Purpose, reason, tc.Reason)
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

// Each row is a null, empty or wrong-type member that was a corpus case with
// the same ID. The shapes share decoders, so each one keeps its own row.
func TestNotificationMemberShapes(t *testing.T) {
	for _, tc := range []struct{ id, body, reason string }{
		{"ENV-006", `{"notificationType":null,"senderDomain":"a.example.org","resourceType":"file"}`, "missing notificationType"},
		{"ENV-010", `{"notificationType":"EXAMPLE_FUTURE","senderDomain":null,"resourceType":"file"}`, "invalid senderDomain"},
		{"ENV-012", `{"notificationType":"EXAMPLE_FUTURE","senderDomain":[],"resourceType":"file"}`, "invalid senderDomain"},
		{"ENV-015", `{"notificationType":"EXAMPLE_FUTURE","senderDomain":"a.example.org","resourceType":null}`, "invalid resourceType"},
		{"ENV-016", `{"notificationType":"EXAMPLE_FUTURE","senderDomain":"a.example.org","resourceType":""}`, "invalid resourceType"},
		{"ENV-017", `{"notificationType":"EXAMPLE_FUTURE","senderDomain":"a.example.org","resourceType":{}}`, "invalid resourceType"},
		{"ENV-018", `{"notificationType":"EXAMPLE_FUTURE","senderDomain":"a.example.org","shareType":null,"notification":{"future":{"providerId":null,"permissions":null,"owner":42,"items":[false,"opaque"],"label":"caf\u00e9","nested":{"x":"anything"}}}}`, "invalid shareType"},
		{"ENV-025", `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":null}},"providerId":"flat-does-not-substitute"}`, "invalid notification.file.providerId"},
		{"ENV-026", `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":""}}}`, "invalid notification.file.providerId"},
		{"ENV-027", `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":42}}}`, "invalid notification.file.providerId"},
		{"ENV-029", `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"share-7","permissions":["read",null]}}}`, "invalid notification.file.permissions"},
		{"ENV-038", `{"notificationType":"GROUP_REMOVED","senderDomain":"a.example.org","shareType":"group","notification":{"group":{"groupId":"group-1"}},"resourceType":null}`, "invalid resourceType"},
		{"ENV-042", `{"notificationType":"EXAMPLE_FUTURE","senderDomain":"a.example.org","resourceType":"file","notification":{"message":null}}`, "invalid notification.message"},
		{"contact-null-target", `{"notificationType":"CONTACT_REMOVED","shareType":"user","senderDomain":"alice.example.org","notification":{"user":{"userId":"alice@alice.example.org","targetUserId":null}}}`, "invalid notification.user.targetUserId"},
		{"contact-empty-target", `{"notificationType":"CONTACT_REMOVED","shareType":"user","senderDomain":"alice.example.org","notification":{"user":{"userId":"alice@alice.example.org","targetUserId":""}}}`, "invalid notification.user.targetUserId"},
		{"contact-resourceType-null", `{"notificationType":"CONTACT_REMOVED","shareType":"user","resourceType":null,"senderDomain":"alice.example.org","notification":{"user":{"userId":"alice@alice.example.org","targetUserId":"bob@bob.example.org"}}}`, "invalid resourceType"},
		{"scalar-root", `"value"`, "malformed notification"},
		{"nonobject-payload", `{"notificationType":"EXAMPLE_CUSTOM_EVENT","senderDomain":"a.example.org","resourceType":"file","notification":[]}`, "invalid notification"},
		{"user-null-payload", `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":null}}`, "invalid notification"},
		{"user-null-id", `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":null}}}`, "invalid notification.user.userId"},
		{"user-null-target-forbidden", `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org","targetUserId":null}}}`, "invalid notification.user.targetUserId"},
		{"group-empty-id", `{"notificationType":"GROUP_REMOVED","senderDomain":"a.example.org","shareType":"group","notification":{"group":{"groupId":""}}}`, "invalid notification.group.groupId"},
		{"group-null-id", `{"notificationType":"GROUP_REMOVED","senderDomain":"a.example.org","shareType":"group","notification":{"group":{"groupId":null}}}`, "invalid notification.group.groupId"},
		{"group-wrong-type-id", `{"notificationType":"GROUP_REMOVED","senderDomain":"a.example.org","shareType":"group","notification":{"group":{"groupId":42}}}`, "invalid notification.group.groupId"},
		{"empty-share-beside-resource", `{"notificationType":"ACME","senderDomain":"a.example.org","resourceType":"file","shareType":""}`, "invalid shareType"},
		{"null-share-beside-resource", `{"notificationType":"ACME","senderDomain":"a.example.org","resourceType":"file","shareType":null}`, "invalid shareType"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			reg := NewNotificationRegistry()
			h := &recordingHandler{}
			for typ := range builtinContracts {
				if err := reg.SetHandler(typ, h); err != nil {
					t.Fatal(err)
				}
			}
			n, err := reg.Parse([]byte(tc.body))
			code, ok := NotificationErrorCodeOf(err)
			if n.Class != NotificationClassMalformed || !ok || code == NotificationErrNotImplemented || NotificationReason(err) != tc.reason {
				t.Fatalf("class %s, error %v, want malformed contract error %q", n.Class, err, tc.reason)
			}
			if out, err := reg.Dispatch(context.Background(), n); out != DispatchIgnored || err != nil || len(h.seen) != 0 {
				t.Fatalf("dispatch %s, error %v, handler calls %d", out, err, len(h.seen))
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
		if code, ok := NotificationErrorCodeOf(err); !ok || code != NotificationErrNotImplemented || n.Class != NotificationClassRequest || !reflect.DeepEqual(n.File, &tc.file) {
			t.Fatalf("request %s file %#v: %v", n.Class, n.File, err)
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

func TestNotificationIdentifierUnicode(t *testing.T) {
	const (
		unshare    = `{"notificationType":"%s","senderDomain":"a.example.org","resourceType":"%s","notification":{"file":{"providerId":"42"}}}`
		custom     = `{"notificationType":"ACME","senderDomain":"a.example.org","shareType":"%s"}`
		user       = `{"notificationType":"USER_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"%s@a.example.org"}}}`
		grant      = `{"notificationType":"SHARE_UNSHARED","senderDomain":"a.example.org","resourceType":"file","notification":{"file":{"providerId":"%s"}}}`
		permission = `{"notificationType":"SHARE_CHANGE_PERMISSION","senderDomain":"a.example.org","resourceType":"folder","notification":{"file":{"providerId":"%s"}}}`
		contact    = `{"notificationType":"CONTACT_REMOVED","senderDomain":"a.example.org","shareType":"user","notification":{"user":{"userId":"alice@a.example.org","targetUserId":"%s@b.example.org"}}}`
	)
	for _, tc := range []struct {
		name, body, reason string
		class              NotificationClass
		calls              int
		field, want        string
	}{
		{"replacement in type", fmt.Sprintf(unshare, `ACME\ufffd`, "file"), "", NotificationClassUnknown, 0, "type", "ACME\uFFFD"},
		{"surrogate in unknown type", fmt.Sprintf(unshare, `ACME\ud800`, "file"), "malformed notification", NotificationClassMalformed, 0, "", ""},
		{"surrogate in known type", fmt.Sprintf(unshare, `SHARE_UNSHARED\udc00`, "file"), "malformed notification", NotificationClassMalformed, 0, "", ""},
		{"replacement in resourceType", fmt.Sprintf(unshare, "SHARE_UNSHARED", `file\ufffd`), "", NotificationClassSpec, 1, "resourceType", "file\uFFFD"},
		{"surrogate in resourceType", fmt.Sprintf(unshare, "SHARE_UNSHARED", `file\ud800`), "invalid resourceType", NotificationClassMalformed, 0, "", ""},
		{"replacement in shareType", fmt.Sprintf(custom, `user\ufffd`), "", NotificationClassUnknown, 0, "shareType", "user\uFFFD"},
		{"surrogate in shareType", fmt.Sprintf(custom, `user\udc00`), "invalid shareType", NotificationClassMalformed, 0, "", ""},
		{"raw ff byte", fmt.Sprintf(user, "\xff"), "malformed notification", NotificationClassMalformed, 0, "", ""},
		{"raw overlong c0 af", fmt.Sprintf(user, "\xc0\xaf"), "malformed notification", NotificationClassMalformed, 0, "", ""},
		{"raw truncated e2 82", fmt.Sprintf(user, "\xe2\x82"), "malformed notification", NotificationClassMalformed, 0, "", ""},
		{"raw replacement in user", fmt.Sprintf(user, "\uFFFD"), "", NotificationClassSpec, 1, "userId", "\uFFFD@a.example.org"},
		{"escaped replacement and pair in user", fmt.Sprintf(user, `\ufffd\uD83D\uDE00`), "", NotificationClassSpec, 1, "userId", "\uFFFD\U0001f600@a.example.org"},
		{"escaped backslash before u in user", fmt.Sprintf(user, `\ufffd\\ud800`), "", NotificationClassSpec, 1, "userId", "\uFFFD" + `\ud800@a.example.org`},
		{"raw accent in user", fmt.Sprintf(user, "\u00c9 @ User"), "", NotificationClassSpec, 1, "userId", "\u00c9 @ User@a.example.org"},
		{"raw replacement in grant", fmt.Sprintf(grant, "\uFFFD"), "", NotificationClassSpec, 1, "providerId", "\uFFFD"},
		{"escaped replacement and pair in grant", fmt.Sprintf(grant, `\ufffd\uD83D\uDE00`), "", NotificationClassSpec, 1, "providerId", "\uFFFD\U0001f600"},
		{"escaped backslash before u in grant", fmt.Sprintf(grant, `\ufffd\\ud800`), "", NotificationClassSpec, 1, "providerId", "\uFFFD" + `\ud800`},
		{"raw accent in grant", fmt.Sprintf(grant, "\u00c9 @ User"), "", NotificationClassSpec, 1, "providerId", "\u00c9 @ User"},
		{"pair before replacement in grant", fmt.Sprintf(grant, `\ud83d\ude00\ufffd`), "", NotificationClassSpec, 1, "providerId", "\U0001f600\uFFFD"},
		{"unpaired high surrogate in grant", fmt.Sprintf(permission, `\ud800`), "invalid notification.file.providerId", NotificationClassMalformed, 0, "", ""},
		{"high surrogate at the end of grant", fmt.Sprintf(grant, `x\ud800`), "invalid notification.file.providerId", NotificationClassMalformed, 0, "", ""},
		{"unpaired low surrogate in address", fmt.Sprintf(contact, `\udc00`), "invalid notification.user.targetUserId", NotificationClassMalformed, 0, "", ""},
		{"interrupted pair in address", fmt.Sprintf(contact, `\ud800x\udc00`), "invalid notification.user.targetUserId", NotificationClassMalformed, 0, "", ""},
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
			got := map[string]string{"type": n.Type, "resourceType": n.ResourceType, "shareType": n.ShareType}
			if n.User != nil {
				got["userId"] = n.User.UserID
			}
			if n.File != nil {
				got["providerId"] = n.File.ProviderID
			}
			if tc.field != "" && got[tc.field] != tc.want {
				t.Fatalf("%s %q, want %q", tc.field, got[tc.field], tc.want)
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
		if strings.Contains(tc.body, key) && (!errors.As(err, &ne) || ne.Code != NotificationErrInvalidField || strings.Contains(ne.field, key)) {
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
		fmt.Errorf("leaky-value: %w", invalidField("notification")),
		errors.New("leaky-value"),
		&NotificationError{Code: NotificationErrorCode("leaky-value"), field: "leaky-value"},
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
