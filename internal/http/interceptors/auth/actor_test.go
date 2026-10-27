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

package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	appctxmw "github.com/cs3org/reva/v3/internal/http/interceptors/appctx"
	logmw "github.com/cs3org/reva/v3/internal/http/interceptors/log"
	authscope "github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/rs/zerolog"
)

func TestRequestLogShowsTheActor(t *testing.T) {
	user := &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein", Idp: "example.org"}, Username: "einstein"}
	run := func(scopes map[string]*authpb.Scope) string {
		var buf bytes.Buffer
		authenticate := func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				h.ServeHTTP(w, r.WithContext(ctxWithUserInfo(r.Context(), r, user, "token", scopes)))
			})
		}
		h := appctxmw.New(zerolog.New(&buf))(logmw.New()(authenticate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/remote.php/webdav/", nil))
		for l := range strings.SplitSeq(buf.String(), "\n") {
			if strings.Contains(l, "processed http request") {
				return l
			}
		}
		t.Fatalf("no request line in %s", buf.String())
		return ""
	}

	owner, err := authscope.AddOwnerScope(nil)
	if err != nil {
		t.Fatal(err)
	}
	if l := run(owner); strings.Contains(l, `"act"`) {
		t.Errorf("login token: the request line shows an actor: %s", l)
	}
	if l := run(authscope.AddActorScope("gdelmont", owner)); !strings.Contains(l, `"act":"gdelmont"`) {
		t.Errorf("impersonated token: the request line has no act: %s", l)
	}
}
