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

package response

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteOCSErrorWithHTTPStatusOverridesOCSV1StatusMapper(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/ocs/v1.php/apps/files_sharing/api/v1/shares/1/notify", nil)
	req = req.WithContext(context.WithValue(req.Context(), apiVersionKey, "1"))
	rec := httptest.NewRecorder()

	WriteOCSErrorWithHTTPStatus(rec, req, http.StatusForbidden, http.StatusForbidden, "forbidden", nil)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("http status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
