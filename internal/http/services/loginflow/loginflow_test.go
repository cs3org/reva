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

package loginflow

import (
	"context"
	"slices"
	"testing"

	apppb "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appauth"
	"github.com/cs3org/reva/v3/pkg/appauth/loginflow"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
)

// recorder is an appauth manager that records what it is asked to create.
type recorder struct {
	appauth.Manager
	owner      *userpb.User
	scopes     map[string]*authpb.Scope
	label      string
	expiration *typespb.Timestamp
}

func (r *recorder) GenerateAppPassword(ctx context.Context, scopes map[string]*authpb.Scope, label string, expiration *typespb.Timestamp) (*apppb.AppPassword, error) {
	r.owner = appctx.ContextMustGetUser(ctx)
	r.scopes, r.label, r.expiration = scopes, label, expiration
	return &apppb.AppPassword{Password: "secret"}, nil
}

func TestMintAppPassword(t *testing.T) {
	owner := &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein", Type: userpb.UserType_USER_TYPE_PRIMARY}, Username: "einstein"}
	cases := []struct {
		ua     string
		client string
		perms  []string
	}{
		{"Mozilla/5.0 (Linux) mirall/3.16.0 (Nextcloud)", scope.ClientSync, []string{scope.PermAll}},
		{"cernbox-cli/1.2.0 (linux)", scope.ClientCLI, scope.ReadWritePermissions()},
	}
	for _, c := range cases {
		am := &recorder{}
		s := &svc{am: am}
		ca := &loginflow.ClientAuthorization{ClientID: "c1", UserAgent: c.ua, DeviceName: "laptop", UserID: "einstein", Username: "einstein"}
		pass, err := s.mintAppPassword(context.Background(), owner, ca)
		if err != nil || pass != "secret" {
			t.Fatalf("%s: mint: %q, %v", c.ua, pass, err)
		}
		ap, ok := scope.GetAppAuthScope(am.scopes)
		if !ok || len(am.scopes) != 1 || ap.Client != c.client || !slices.Equal(ap.Permissions, c.perms) || len(ap.Roots) != 0 {
			t.Errorf("%s: scope %+v", c.ua, ap)
		}
		// The store applies the default expiry to a CLI, none to a sync client.
		if am.expiration != nil {
			t.Errorf("%s: expiration %v", c.ua, am.expiration)
		}
		if am.owner.GetId().GetType() != userpb.UserType_USER_TYPE_PRIMARY {
			t.Errorf("%s: the store must see the user type, got %+v", c.ua, am.owner)
		}
		if am.label != "laptop|"+loginflow.ClientDescription(c.ua)+"|c1" {
			t.Errorf("%s: label %q", c.ua, am.label)
		}
	}
}
