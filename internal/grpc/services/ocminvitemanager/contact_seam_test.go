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

package ocminvitemanager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/ocm/invite"
)

type deleteOnlyRepo struct {
	invite.Repository
	deleted int
}

func (r *deleteOnlyRepo) DeleteRemoteUser(context.Context, *userpb.UserId, *userpb.UserId) error {
	r.deleted++
	return nil
}

// Removing a contact stays local in this layer: no notification is sent.
func TestDeleteAcceptedUserSendsNothing(t *testing.T) {
	var requests atomic.Int32
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	defer peer.Close()

	repo := &deleteOnlyRepo{}
	s := &service{conf: &config{}, repo: repo, ocmClient: ocmd.NewClient(time.Second, false)}
	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{Id: &userpb.UserId{OpaqueId: "alice", Idp: "a.example.org"}})
	res, err := s.DeleteAcceptedUser(ctx, &invitepb.DeleteAcceptedUserRequest{
		RemoteUserId: &userpb.UserId{OpaqueId: "bob", Idp: peer.Listener.Addr().String()},
	})
	if err != nil || res.Status.Code != rpc.Code_CODE_OK || repo.deleted != 1 {
		t.Fatalf("%v %v deleted=%d", res, err, repo.deleted)
	}
	if requests.Load() != 0 {
		t.Fatalf("contact removal sent %d requests", requests.Load())
	}
}
