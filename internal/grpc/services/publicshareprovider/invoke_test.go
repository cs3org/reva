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

package publicshareprovider

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/publicshare/manager/memory"
	sqlmgr "github.com/cs3org/reva/v3/pkg/share/manager/sql"
)

var einstein = &userpb.User{Id: &userpb.UserId{OpaqueId: "einstein", Idp: "cernbox"}, Username: "einstein"}

func seed(t *testing.T, s *service, inode string, expires time.Time) string {
	t.Helper()
	md := &provider.ResourceInfo{
		Id:    &provider.ResourceId{StorageId: "eoshome-e", OpaqueId: inode},
		Path:  "/eos/user/e/einstein/" + inode,
		Type:  provider.ResourceType_RESOURCE_TYPE_CONTAINER,
		Owner: einstein.Id,
	}
	grant := &link.Grant{
		Permissions: &link.PublicSharePermissions{Permissions: &provider.ResourcePermissions{Stat: true}},
		Expiration:  &typespb.Timestamp{Seconds: uint64(expires.Unix())},
	}
	l, err := s.sm.CreatePublicShare(appctx.ContextSetUser(context.Background(), einstein), einstein, md, grant, "", false, false, "")
	if err != nil {
		t.Fatal(err)
	}
	return l.Token
}

func TestExpiringPagesByExpirationAndID(t *testing.T) {
	sm, err := sqlmgr.NewPublicShareManager(context.Background(), map[string]any{"db_engine": "sqlite", "db_name": filepath.Join(t.TempDir(), "links.db")})
	if err != nil {
		t.Fatal(err)
	}
	s := &service{conf: &config{Driver: "sql"}, sm: sm}
	s.registerInvocations()

	now := time.Now().Truncate(time.Second)
	day := 24 * time.Hour
	want := []string{seed(t, s, "a", now.Add(3*day)), seed(t, s, "b", now.Add(3*day)), seed(t, s, "c", now.Add(5*day))}
	seed(t, s, "d", now.Add(10*day))

	// the arguments arrive as strings over the control channel
	args := map[string]any{"from": now.Format(time.RFC3339), "to": now.Add(7 * day).Format(time.RFC3339), "limit": "2"}
	var got []string
	for range 3 {
		res, err := s.Invoke(context.Background(), "expiring", args)
		if err != nil {
			t.Fatal(err)
		}
		links := res["links"].([]map[string]any)
		if len(links) == 0 {
			break
		}
		for _, l := range links {
			got = append(got, l["token"].(string))
		}
		last := links[len(links)-1]
		args["after_expiration"], args["after_id"] = last["expires_at"], fmt.Sprint(last["id"])
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExpiringRefusesBadArguments(t *testing.T) {
	sm, err := memory.New(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := &service{conf: &config{Driver: "memory"}, sm: sm}
	s.registerInvocations()
	for _, args := range []map[string]any{
		{"from": "2026-01-01T00:00:00Z", "to": "2026-01-02T00:00:00Z"},
		{"from": "yesterday", "to": "2026-01-02T00:00:00Z"},
		{"from": "2026-01-02T00:00:00Z", "to": "2026-01-01T00:00:00Z"},
	} {
		if _, err := s.Invoke(context.Background(), "expiring", args); err == nil {
			t.Errorf("%v must be refused", args)
		}
	}
}
