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

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	authscope "github.com/cs3org/reva/v3/pkg/auth/scope"
	jwt "github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/metadata"
)

func TestCtxWithUserInfoStoresScopes(t *testing.T) {
	user := &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein", Idp: "example.org"}}
	scopes, err := authscope.AddOwnerScope(nil)
	if err != nil {
		t.Fatalf("AddOwnerScope returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.org/remote.php/dav/files/test.txt", nil)
	req.Header.Set("User-Agent", "dav-client")

	ctx := ctxWithUserInfo(context.Background(), req, user, "token-123", scopes)

	gotUser, ok := appctx.ContextGetUser(ctx)
	if !ok || gotUser.GetId().GetOpaqueId() != "einstein" {
		t.Fatalf("ContextGetUser() = %+v, %t, want einstein", gotUser, ok)
	}
	gotToken, ok := appctx.ContextGetToken(ctx)
	if !ok || gotToken != "token-123" {
		t.Fatalf("ContextGetToken() = %q, %t, want token-123", gotToken, ok)
	}
	gotScopes, ok := appctx.ContextGetScopes(ctx)
	if !ok || !reflect.DeepEqual(gotScopes, scopes) {
		t.Fatalf("ContextGetScopes() = %+v, %t, want %+v", gotScopes, ok, scopes)
	}

	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata in context")
	}
	if got := md.Get(appctx.TokenHeader); len(got) != 1 || got[0] != "token-123" {
		t.Fatalf("outgoing %s metadata = %v, want [token-123]", appctx.TokenHeader, got)
	}
	if got := md.Get(appctx.UserAgentHeader); len(got) != 1 || got[0] != "dav-client" {
		t.Fatalf("outgoing %s metadata = %v, want [dav-client]", appctx.UserAgentHeader, got)
	}
}

func TestIsTokenValidReturnsScopes(t *testing.T) {
	tokenManager, err := jwt.New(map[string]any{
		"secret":  "test-secret-auth",
		"expires": int64(3600),
	})
	if err != nil {
		t.Fatalf("jwt.New returned error: %v", err)
	}

	user := &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein", Idp: "example.org"}}
	scopes, err := authscope.AddOwnerScope(nil)
	if err != nil {
		t.Fatalf("AddOwnerScope returned error: %v", err)
	}

	token, err := tokenManager.MintToken(context.Background(), user, scopes)
	if err != nil {
		t.Fatalf("MintToken returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.org/remote.php/dav/ocm/share123", nil)
	gotUser, gotScopes, ok := isTokenValid(req, tokenManager, token)
	if !ok {
		t.Fatal("isTokenValid() returned false")
	}
	if gotUser.GetId().GetOpaqueId() != "einstein" {
		t.Fatalf("isTokenValid() user = %q, want einstein", gotUser.GetId().GetOpaqueId())
	}
	if !reflect.DeepEqual(gotScopes, scopes) {
		t.Fatalf("isTokenValid() scopes = %+v, want %+v", gotScopes, scopes)
	}
}

func TestGetCredsForUserAgent(t *testing.T) {
	type test struct {
		userAgent            string
		userAgentMap         map[string]string
		availableCredentials []string
		expected             []string
	}

	tests := []*test{
		// no user agent we return all available credentials
		{
			userAgent:            "",
			userAgentMap:         map[string]string{},
			availableCredentials: []string{"basic"},
			expected:             []string{"basic"},
		},

		// map set but user agent not in map
		{
			userAgent:            "curl",
			userAgentMap:         map[string]string{"mirall": "basic"},
			availableCredentials: []string{"basic", "bearer"},
			expected:             []string{"basic", "bearer"},
		},

		// no user map we return all available credentials
		{
			userAgent:            "mirall",
			userAgentMap:         map[string]string{},
			availableCredentials: []string{"basic"},
			expected:             []string{"basic"},
		},

		// user agent set but no mapping set we return all credentials
		{
			userAgent:            "mirall",
			userAgentMap:         map[string]string{},
			availableCredentials: []string{"basic"},
			expected:             []string{"basic"},
		},

		// user mapping set to non available credential, we return all available
		{
			userAgent:            "mirall",
			userAgentMap:         map[string]string{"mirall": "notfound"},
			availableCredentials: []string{"basic", "bearer"},
			expected:             []string{"basic", "bearer"},
		},

		// user mapping set and we return only desired credential
		{
			userAgent:            "mirall",
			userAgentMap:         map[string]string{"mirall": "bearer"},
			availableCredentials: []string{"basic", "bearer"},
			expected:             []string{"bearer"},
		},
	}

	for _, test := range tests {
		got := getCredsForUserAgent(
			test.userAgent,
			test.userAgentMap,
			test.availableCredentials)

		if !match(got, test.expected) {
			fail(t, got, test.expected)
		}
	}
}

func match(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func fail(t *testing.T, got, expected []string) {
	t.Fatalf("got: %+v expected: %+v", got, expected)
}

func TestWriteIdentityAuthConflictHTTPResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	log := zerolog.New(io.Discard)
	err := errors.New("rpc: conflict")

	writeIdentityAuthConflictHTTPResponse(false, rec, &log, err)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if got := rec.Header().Get(headerLinkedPrimaryAccount); got != "true" {
		t.Fatalf("X-Oc-Linked-Primary-Account = %q, want true", got)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if jsonErr := json.Unmarshal(rec.Body.Bytes(), &body); jsonErr != nil {
		t.Fatalf("unmarshal body: %v", jsonErr)
	}
	if !bytes.Equal(rec.Body.Bytes(), linkedPrimaryErrorJSONBody) {
		t.Fatalf("response body differs from packaged linkedPrimaryErrorJSONBody")
	}
	if body.Error.Code != jsonLinkedPrimaryErrorCode {
		t.Fatalf("error.code = %q, want %q", body.Error.Code, jsonLinkedPrimaryErrorCode)
	}
	msg := strings.ToLower(body.Error.Message)
	if !strings.Contains(msg, "linked primary account") {
		t.Fatalf("error.message = %q, should mention linked primary account", body.Error.Message)
	}
}

func TestWriteIdentityAuthConflictHTTPResponse_UnprotectedNoResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	log := zerolog.New(io.Discard)

	writeIdentityAuthConflictHTTPResponse(true, rec, &log, errors.New("rpc: conflict"))

	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty body, got %q", rec.Body.String())
	}
	if got := rec.Header().Get(headerLinkedPrimaryAccount); got != "" {
		t.Fatalf("expected no linked-primary header when unprotected, got %q", got)
	}
}

func TestWriteIdentityAuthConflictHTTPResponse_NilLogger(t *testing.T) {
	rec := httptest.NewRecorder()

	writeIdentityAuthConflictHTTPResponse(false, rec, nil, errors.New("rpc: conflict"))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("expected JSON body")
	}
	if !bytes.Equal(rec.Body.Bytes(), linkedPrimaryErrorJSONBody) {
		t.Fatal("nil-logger response body differs from packaged linkedPrimaryErrorJSONBody")
	}
}

func TestWriteIdentityAuthConflictHTTPResponse_NilCauseError(t *testing.T) {
	rec := httptest.NewRecorder()
	log := zerolog.New(io.Discard)

	writeIdentityAuthConflictHTTPResponse(false, rec, &log, nil)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if !bytes.Equal(rec.Body.Bytes(), linkedPrimaryErrorJSONBody) {
		t.Fatal("nil-cause response body differs from packaged linkedPrimaryErrorJSONBody")
	}
}

func TestLinkedPrimaryErrorJSONBodyShape(t *testing.T) {
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(linkedPrimaryErrorJSONBody, &body); err != nil {
		t.Fatalf("unmarshal init body: %v", err)
	}
	if body.Error.Code != jsonLinkedPrimaryErrorCode {
		t.Fatalf("code %q want %q", body.Error.Code, jsonLinkedPrimaryErrorCode)
	}
	if !strings.Contains(strings.ToLower(body.Error.Message), "linked primary account") {
		t.Fatalf("message fragment: %q", body.Error.Message)
	}
}

// responseWriterWriteFail simulates a broken client connection mid-body.
type responseWriterWriteFail struct {
	hdr          http.Header
	wroteHeaders bool
	code         int
}

func (w *responseWriterWriteFail) Header() http.Header {
	if w.hdr == nil {
		w.hdr = make(http.Header)
	}
	return w.hdr
}

func (w *responseWriterWriteFail) WriteHeader(statusCode int) {
	w.wroteHeaders = true
	w.code = statusCode
}

func (w *responseWriterWriteFail) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestWriteIdentityAuthConflictHTTPResponse_WriteFailsNoPanic(t *testing.T) {
	log := zerolog.New(io.Discard)
	w := &responseWriterWriteFail{}

	writeIdentityAuthConflictHTTPResponse(false, w, &log, errors.New("rpc: conflict"))

	if !w.wroteHeaders || w.code != http.StatusConflict {
		t.Fatalf("headers: wroteHeaders=%v code=%d, want conflict", w.wroteHeaders, w.code)
	}
	if w.Header().Get(headerLinkedPrimaryAccount) != "true" {
		t.Fatal("missing linked-primary header")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q want no-store", w.Header().Get("Cache-Control"))
	}
}

// When the contextual logger is nil and the body write fails, the discard logger path must not panic.
func TestWriteIdentityAuthConflictHTTPResponse_WriteFailsNilLogger(t *testing.T) {
	w := &responseWriterWriteFail{}

	writeIdentityAuthConflictHTTPResponse(false, w, nil, errors.New("rpc: conflict"))

	if w.code != http.StatusConflict {
		t.Fatalf("status = %d", w.code)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", w.Header().Get("Cache-Control"))
	}
}

func TestWriteIdentityAuthConflictHTTPResponse_UnprotectedNilLogger(t *testing.T) {
	rec := httptest.NewRecorder()

	writeIdentityAuthConflictHTTPResponse(true, rec, nil, errors.New("rpc: conflict"))

	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty body, got %q", rec.Body.String())
	}
	if rec.Header().Get(headerLinkedPrimaryAccount) != "" {
		t.Fatalf("unexpected header: %q", rec.Header().Get(headerLinkedPrimaryAccount))
	}
	if rec.Header().Get("Cache-Control") != "" {
		t.Fatalf("unexpected Cache-Control: %q", rec.Header().Get("Cache-Control"))
	}
}

func TestLinkedPrimaryErrorJSONBodyIsValidUTF8(t *testing.T) {
	if !utf8.Valid(linkedPrimaryErrorJSONBody) {
		t.Fatal("linkedPrimaryErrorJSONBody is not valid UTF-8")
	}
}

func TestLinkedPrimaryErrorJSONBodyStrictShape(t *testing.T) {
	dec := json.NewDecoder(bytes.NewReader(linkedPrimaryErrorJSONBody))
	dec.DisallowUnknownFields()
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := dec.Decode(&envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if dec.More() {
		t.Fatal("linkedPrimaryErrorJSONBody must be a single JSON value with no trailing data")
	}
	if envelope.Error.Code != jsonLinkedPrimaryErrorCode {
		t.Fatalf("code %q", envelope.Error.Code)
	}
	if envelope.Error.Message != jsonLinkedPrimaryErrorMessageEnglish {
		t.Fatalf("message mismatch")
	}
}

func TestLinkedPrimaryErrorJSONBodyTopLevelKeys(t *testing.T) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(linkedPrimaryErrorJSONBody, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(raw) != 1 {
		t.Fatalf("expected exactly one top-level key, got %d: %v", len(raw), raw)
	}
	if _, ok := raw["error"]; !ok {
		t.Fatalf("expected top-level key error, got %v", raw)
	}
}
