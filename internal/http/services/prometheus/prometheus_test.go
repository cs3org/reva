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

package prometheus

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/pkg/logger"
)

func TestServesLogMessagesCounter(t *testing.T) {
	logger.New(logger.WithWriter(io.Discard, logger.JSONMode)).Error().Msg("e")
	for want, conf := range map[string]map[string]any{
		`reva_log_messages_total{level="error"} 1`:   {},
		`cernbox_log_messages_total{level="error"} 1`: {"logs": map[string]any{"namespace": "cernbox"}},
	} {
		s, err := New(context.Background(), conf)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		s.(*svc).h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s missing from /metrics:\n%s", want, rec.Body.String())
		}
	}
}
