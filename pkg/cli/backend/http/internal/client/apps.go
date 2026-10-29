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
	"net/http"
	"net/url"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

const appOpenPath = "/app/open"

// AppSession is the answer of the server to an open request.
type AppSession struct {
	URL            string            `json:"url"`
	Method         string            `json:"method,omitempty"`
	FormParameters map[string]string `json:"form_parameters,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
}

type appOpenResponse struct {
	AppURL         string            `json:"app_url"`
	Method         string            `json:"method"`
	FormParameters map[string]string `json:"form_parameters"`
	Headers        map[string]string `json:"headers"`
}

// OpenInApp asks the server for a session to open a resource in a web
// application. An empty appName takes the default app of the file type.
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
