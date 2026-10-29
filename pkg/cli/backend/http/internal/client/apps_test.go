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

	session, err := f.client().OpenInApp(context.Background(), "s1$ABC!item-1", "Collabora", "write")
	if err != nil {
		t.Fatal(err)
	}
	gotForm := recordedForm(t, f)
	if session.URL != "https://office.cern.ch/edit?wopi=abc" {
		t.Errorf("URL = %q", session.URL)
	}

	if gotForm.Get("file_id") != "s1$ABC!item-1" {
		t.Errorf("file_id = %q", gotForm.Get("file_id"))
	}
	if gotForm.Get("app_name") != "Collabora" {
		t.Errorf("app_name = %q", gotForm.Get("app_name"))
	}
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
	if _, present := gotForm["app_name"]; present {
		t.Error("app_name should be omitted when no application was requested")
	}
	if _, present := gotForm["view_mode"]; present {
		t.Error("view_mode should be omitted when unset")
	}
}

func TestOpenInAppKeepsFormParameters(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodPost, appOpenPath, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"app_url":"https://office.cern.ch/wopi","method":"POST",
		  "form_parameters":{"access_token":"secret"}}`)
	})

	session, err := f.client().OpenInApp(context.Background(), "s1$ABC!item-1", "", "")
	if err != nil {
		t.Fatal(err)
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

// recordedForm decodes the form body of the most recent POST.
func recordedForm(t *testing.T, f *fakeServer) url.Values {
	t.Helper()
	values, err := url.ParseQuery(f.lastRequest(http.MethodPost).Body)
	if err != nil {
		t.Fatalf("request body is not a form: %v", err)
	}
	return values
}
