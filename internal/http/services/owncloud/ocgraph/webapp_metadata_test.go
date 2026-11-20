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

package ocgraph

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"

	"github.com/cs3org/reva/v3/pkg/appctx"
)

func TestGetSharedWithMeEmptyValue(t *testing.T) {
	stampGateway(&receivedSharesGateway{})
	s := &svc{c: &config{OCMEnabled: false}}
	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{OpaqueId: "me"},
	})
	req := httptest.NewRequest(http.MethodGet, "/me/drive/sharedWithMe", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	s.getSharedWithMe(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}

	var want bytes.Buffer
	if err := encodeSharedWithMe(&want, []any{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rec.Body.Bytes(), want.Bytes()) {
		t.Fatalf("empty body = %s, want %s", rec.Body.Bytes(), want.Bytes())
	}
}
