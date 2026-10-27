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

package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

func TestOpenInApp(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodPost, appOpenPath, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"app_url":"https://office.cern.ch/edit?wopi=abc","method":"GET"}`)
	})

	session, err := f.client().OpenInApp(context.Background(), "s1$ABC!item-1", "Collabora", ViewModeWrite)
	if err != nil {
		t.Fatal(err)
	}
	// The fake server drains the body before dispatching, so read the form back
	// from the recorded request rather than from r.Form.
	gotForm := recordedForm(t, f)
	if session.URL != "https://office.cern.ch/edit?wopi=abc" {
		t.Errorf("URL = %q", session.URL)
	}
	if !session.OpenableInBrowser() {
		t.Error("a GET session should be openable by pasting the link")
	}

	if gotForm.Get("file_id") != "s1$ABC!item-1" {
		t.Errorf("file_id = %q", gotForm.Get("file_id"))
	}
	if gotForm.Get("app_name") != "Collabora" {
		t.Errorf("app_name = %q", gotForm.Get("app_name"))
	}
	// A literal, not the constant: comparing the constant with itself is how
	// sending the CS3 enum name, which reva rejects, went unnoticed.
	if gotForm.Get("view_mode") != "write" {
		t.Errorf("view_mode = %q", gotForm.Get("view_mode"))
	}
}

func TestOpenInAppOmitsUnsetParameters(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodPost, appOpenPath, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"app_url":"https://office.cern.ch/view","method":"GET"}`)
	})

	if _, err := f.client().OpenInApp(context.Background(), "s1$ABC!item-1", "", ""); err != nil {
		t.Fatal(err)
	}
	gotForm := recordedForm(t, f)
	// Sending an empty app_name would ask the server for an application
	// literally named "", rather than letting it pick the default.
	if _, present := gotForm["app_name"]; present {
		t.Error("app_name should be omitted when no application was requested")
	}
	if _, present := gotForm["view_mode"]; present {
		t.Error("view_mode should be omitted when unset")
	}
}

// TestOpenInAppPostSessionIsNotALink: a POST session needs form parameters the
// browser must submit, so pasting the URL would not work and the CLI has to
// know the difference.
func TestOpenInAppPostSessionIsNotALink(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodPost, appOpenPath, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"app_url":"https://office.cern.ch/wopi","method":"POST",
		  "form_parameters":{"access_token":"secret"}}`)
	})

	session, err := f.client().OpenInApp(context.Background(), "s1$ABC!item-1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if session.OpenableInBrowser() {
		t.Error("a POST session must not be reported as a plain link")
	}
	if session.FormParameters["access_token"] != "secret" {
		t.Errorf("form parameters = %+v", session.FormParameters)
	}
}

func TestOpenInAppWithoutURL(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodPost, appOpenPath, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{}`)
	})

	_, err := f.client().OpenInApp(context.Background(), "s1$ABC!item-1", "", "")
	if err == nil || !strings.Contains(err.Error(), "no application URL") {
		t.Errorf("got %v, want a clear error", err)
	}
}

func TestOpenInAppRequiresResourceID(t *testing.T) {
	f := newFakeServer(t)
	_, err := f.client().OpenInApp(context.Background(), "", "", "")
	if cberr.KindOf(err) != cberr.KindUsage {
		t.Errorf("got %v, want a usage error", err)
	}
}

func TestListApps(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodGet, appListPath, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"mime-types":[
		  {"mime_type":"application/vnd.oasis.opendocument.text","ext":"odt","name":"OpenDocument",
		   "allow_creation":true,
		   "default_application":"Collabora",
		   "app_providers":[{"name":"Collabora"},{"name":"OnlyOffice"}]},
		  {"mime_type":"text/plain","ext":"txt","app_providers":[]}
		]}`)
	})

	types, err := f.client().ListApps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(types) != 2 {
		t.Fatalf("got %d types, want 2", len(types))
	}
	first := types[0]
	if first.Extension != "odt" || first.Default != "Collabora" {
		t.Errorf("type = %+v", first)
	}
	if len(first.Apps) != 2 {
		t.Errorf("apps = %v, want both providers", first.Apps)
	}
	if !first.AllowCreation {
		t.Error("allow_creation was lost")
	}
	if len(types[1].Apps) != 0 {
		t.Errorf("a type with no providers should have no apps: %+v", types[1])
	}
}

func TestOpenableInBrowser(t *testing.T) {
	tests := []struct {
		name    string
		session *AppSession
		want    bool
	}{
		{"nil", nil, false},
		{"no URL", &AppSession{Method: "GET"}, false},
		{"GET", &AppSession{URL: "https://x", Method: "GET"}, true},
		{"lowercase get", &AppSession{URL: "https://x", Method: "get"}, true},
		{"unspecified method", &AppSession{URL: "https://x"}, true},
		{"POST", &AppSession{URL: "https://x", Method: "POST"}, false},
	}
	for _, tt := range tests {
		if got := tt.session.OpenableInBrowser(); got != tt.want {
			t.Errorf("%s: OpenableInBrowser() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// recordedForm decodes the form body of the most recent POST.
func recordedForm(t *testing.T, f *fakeServer) url.Values {
	t.Helper()
	values, err := url.ParseQuery(f.lastRequest(http.MethodPost).Body)
	if err != nil {
		t.Fatalf("request body is not a form: %v", err)
	}
	return values
}
