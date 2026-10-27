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
	"net/http"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

// ServerTime returns the time of the server, from the Date header. The
// request has no credentials and accepts any status: a credential depends on
// the clock (Kerberos refuses a ticket that is 5 minutes off), so the clock
// must be readable first, and a 401 has a Date too.
func (c *Client) ServerTime(ctx context.Context) (time.Time, error) {
	resp, err := c.do(ctx, request{
		method:    http.MethodHead,
		url:       c.URL("/"),
		op:        "read the server clock",
		noAuth:    true,
		anyStatus: true,
	})
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()

	v := resp.Header.Get("Date")
	if v == "" {
		return time.Time{}, cberr.New(cberr.KindOther, "read the server clock", "",
			"the server sent no Date header")
	}
	t, err := http.ParseTime(v)
	if err != nil {
		return time.Time{}, cberr.Wrap(cberr.KindOther, "read the server clock", "", err)
	}
	return t, nil
}
