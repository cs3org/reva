// Copyright 2018-2022 CERN
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

package antivirus

import (
	"io"
	"net/http"
	"net/url"
	"time"

	ic "github.com/egirna/icap-client"
)

// NewICAP returns a Scanner talking to an ICAP server
func NewICAP(icapURL string, icapService string, timeout time.Duration) (ICAP, error) {
	endpoint, err := url.Parse(icapURL)
	if err != nil {
		return ICAP{}, err
	}

	endpoint.Scheme = "icap"
	endpoint.Path = icapService

	return ICAP{
		client: &ic.Client{
			Timeout: timeout,
		},
		endpoint: endpoint.String(),
	}, nil
}

// ICAP is a Scanner talking to an ICAP server
type ICAP struct {
	client   *ic.Client
	endpoint string
}

// Scan to fulfill Scanner interface
func (s ICAP) Scan(file io.Reader) (ScanResult, error) {
	httpReq, err := http.NewRequest(http.MethodGet, "http://localhost", file)
	if err != nil {
		return ScanResult{}, err
	}

	req, err := ic.NewRequest(ic.MethodREQMOD, s.endpoint, httpReq, nil)
	if err != nil {
		return ScanResult{}, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return ScanResult{}, err
	}

	_, infected := resp.Header["X-Infection-Found"]

	return ScanResult{
		Infected: infected,
	}, nil
}
