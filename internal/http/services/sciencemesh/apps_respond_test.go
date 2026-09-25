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

package sciencemesh

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

func TestRedactLaunchError(t *testing.T) {
	secret := launchSecret
	token := launchToken
	mfa := errtypes.PermissionDenied(ocmd.ErrWebappMFAUnproven.Error())
	tests := []struct {
		name    string
		err     error
		want    string
		keep    error
		typedAs string
	}{
		{
			name:    "internal remote url collapses",
			err:     errtypes.InternalError("upstream https://token.example/ocm said " + token),
			want:    "launch failed",
			typedAs: "internal",
		},
		{
			name:    "unknown gateway text collapses",
			err:     errtypes.InternalError("gateway down " + secret),
			want:    "launch failed",
			typedAs: "internal",
		},
		{
			name:    "fixed missing share response stays",
			err:     errtypes.InternalError("missing share response"),
			want:    "missing share response",
			typedAs: "internal",
		},
		{
			name:    "bad request keeps a safe detail",
			err:     errtypes.BadRequest("malformed remote URL"),
			want:    "malformed remote URL",
			typedAs: "bad",
		},
		{
			name:    "bad request drops secret and token",
			err:     errtypes.BadRequest("rejected " + secret + " " + token),
			want:    "invalid parameter",
			typedAs: "bad",
		},
		{
			name:    "mfa sentence stays",
			err:     mfa,
			want:    ocmd.ErrWebappMFAUnproven.Error(),
			typedAs: "denied",
		},
		{
			name: "canceled context stays observable",
			err:  context.Canceled,
			keep: context.Canceled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactLaunchError(tt.err, secret, token)
			if tt.keep != nil {
				if got != tt.keep {
					t.Fatalf("got %v", got)
				}
				return
			}
			if got == nil || !strings.Contains(got.Error(), tt.want) {
				t.Fatalf("got %v", got)
			}
			if strings.Contains(got.Error(), secret) || strings.Contains(got.Error(), token) || strings.Contains(got.Error(), "token.example") {
				t.Fatalf("leaked %v", got)
			}
			switch tt.typedAs {
			case "internal":
				if _, ok := got.(errtypes.InternalError); !ok {
					t.Fatalf("type %T", got)
				}
			case "bad":
				if _, ok := got.(errtypes.BadRequest); !ok {
					t.Fatalf("type %T", got)
				}
			case "denied":
				if _, ok := got.(errtypes.PermissionDenied); !ok {
					t.Fatalf("type %T", got)
				}
			}
		})
	}
}

func TestWriteLaunchJSONEncoderFailure(t *testing.T) {
	rec := httptest.NewRecorder()
	req, _ := newLaunchRequest(t, "/ocm/share-1")
	writeLaunchJSON(rec, req, make(chan int))
	if rec.Code == http.StatusOK {
		t.Fatal("encoder failure committed HTTP 200")
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "app_url") || strings.Contains(rec.Body.String(), launchToken) {
		t.Fatalf("body %s", rec.Body.String())
	}
	var apiErr reqres.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatal(err)
	}
	if apiErr.Code != reqres.APIErrorServerError {
		t.Fatalf("code %s", apiErr.Code)
	}
}

func TestWriteLaunchJSONWriterFailure(t *testing.T) {
	req, logs := newLaunchRequest(t, "/ocm/share-1")
	w := &failingResponseWriter{header: make(http.Header)}
	writeLaunchJSON(w, req, openInAppResponse{
		AppURL:      "https://app.example/hub",
		AccessToken: launchToken,
	})
	if w.status != http.StatusOK {
		t.Fatalf("status %d", w.status)
	}
	if w.writes != 1 {
		t.Fatalf("writes %d body %s", w.writes, w.body)
	}
	if strings.Contains(w.body, "SERVER_ERROR") {
		t.Fatalf("second error was appended: %s", w.body)
	}
	if !strings.Contains(logs.String(), "error writing launch response") {
		t.Fatalf("log %q", logs.String())
	}
	assertLogsClean(t, logs.String(), launchSecret, launchToken)
}
