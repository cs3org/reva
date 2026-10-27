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
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

const (
	appOpenPath = "/app/open"
	appListPath = "/app/list"
)

// ViewMode selects how an application should open a file. These are the words
// reva's /app/open handler parses, not the CS3 enum names: it maps anything it
// does not recognise to VIEW_MODE_INVALID and answers that with a 401
// "permission denied when accessing the file", so a wrong spelling here reads
// as an authentication failure rather than a bad request.
const (
	ViewModeRead  = "read"
	ViewModeWrite = "write"
)

// AppSession is what the server returns for an open request: everything a
// browser needs to reach the editor.
type AppSession struct {
	// URL is the application endpoint.
	URL string `json:"url"`
	// Method is the HTTP method the browser should use, usually GET or POST.
	Method string `json:"method,omitempty"`
	// FormParameters are posted to URL when Method is POST. Office
	// integrations use this to pass the session token.
	FormParameters map[string]string `json:"form_parameters,omitempty"`
	// Headers are extra headers the browser should send.
	Headers map[string]string `json:"headers,omitempty"`
}

// OpenableInBrowser reports whether visiting URL directly is enough. A POST
// session cannot be opened by pasting a link, so the CLI must say so rather
// than print something that will not work.
func (s *AppSession) OpenableInBrowser() bool {
	return s != nil && s.URL != "" && (s.Method == "" || strings.EqualFold(s.Method, http.MethodGet))
}

type appOpenResponse struct {
	AppURL         string            `json:"app_url"`
	Method         string            `json:"method"`
	FormParameters map[string]string `json:"form_parameters"`
	Headers        map[string]string `json:"headers"`
}

// OpenInApp asks the server for a session to open a resource in a web
// application. appName selects a specific application; empty takes the default
// for the file's type.
func (c *Client) OpenInApp(ctx context.Context, resourceID, appName, viewMode string) (*AppSession, error) {
	if resourceID == "" {
		return nil, cberr.Usagef("no resource id for the file to open")
	}

	form := url.Values{"file_id": {resourceID}}
	if appName != "" {
		form.Set("app_name", appName)
	}
	if viewMode != "" {
		form.Set("view_mode", viewMode)
	}

	resp, err := c.do(ctx, request{
		method: http.MethodPost,
		url:    c.URL(appOpenPath),
		header: http.Header{
			"Content-Type": []string{"application/x-www-form-urlencoded"},
			"Accept":       []string{"application/json"},
		},
		body: stringBody(form.Encode()),
		op:   "open in an application",
		path: resourceID,
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var raw appOpenResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, cberr.Wrap(cberr.KindOther, "open in an application", resourceID, err)
	}
	if raw.AppURL == "" {
		return nil, cberr.New(cberr.KindOther, "open in an application", resourceID,
			"the server returned no application URL")
	}
	return &AppSession{
		URL:            raw.AppURL,
		Method:         raw.Method,
		FormParameters: raw.FormParameters,
		Headers:        raw.Headers,
	}, nil
}

// AppMimeType describes the applications that can handle one file type.
type AppMimeType struct {
	MimeType    string   `json:"mime_type"`
	Extension   string   `json:"extension,omitempty"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Apps        []string `json:"apps,omitempty"`
	Default     string   `json:"default_app,omitempty"`
	// AllowCreation reports whether new files of this type can be created
	// through an application.
	AllowCreation bool `json:"allow_creation,omitempty"`
}

type appListResponse struct {
	MimeTypes []struct {
		MimeType      string `json:"mime_type"`
		Ext           string `json:"ext"`
		Name          string `json:"name"`
		Description   string `json:"description"`
		AllowCreation bool   `json:"allow_creation"`
		// DefaultApp is a plain application name, not an object with one in it.
		// The server marshals the CS3 MimeTypeInfo message straight to JSON and
		// its default_application field is a string, so expecting an object made
		// "cernbox apps" fail outright against a real instance: and the fake
		// here answered with the object, so nothing caught it.
		DefaultApp   string `json:"default_application"`
		AppProviders []struct {
			Name string `json:"name"`
		} `json:"app_providers"`
	} `json:"mime-types"`
}

// ListApps returns the file types the server can open in a web application.
func (c *Client) ListApps(ctx context.Context) ([]AppMimeType, error) {
	resp, err := c.do(ctx, request{
		method: http.MethodGet,
		url:    c.URL(appListPath),
		header: http.Header{"Accept": []string{"application/json"}},
		op:     "list applications",
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, cberr.Wrap(cberr.KindOther, "list applications", "", err)
	}
	var raw appListResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, cberr.Wrap(cberr.KindOther, "list applications", "", err)
	}

	out := make([]AppMimeType, 0, len(raw.MimeTypes))
	for _, m := range raw.MimeTypes {
		item := AppMimeType{
			MimeType:      m.MimeType,
			Extension:     m.Ext,
			Name:          m.Name,
			Description:   m.Description,
			AllowCreation: m.AllowCreation,
		}
		item.Default = m.DefaultApp
		for _, p := range m.AppProviders {
			if p.Name != "" {
				item.Apps = append(item.Apps, p.Name)
			}
		}
		out = append(out, item)
	}
	return out, nil
}
