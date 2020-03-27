// Copyright 2018-2020 CERN
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

package memory

import (
	"sync"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
)

func Test_mgr_SharesWorkflow(t *testing.T) {

	managerWithData := createManagerWithData()
	user := userpb.User{
		Id: &userpb.UserId{
			Idp:      "http://localhost:20080",
			OpaqueId: "4c510ada-c86b-4815-8820-42cdf82c3d51",
		},
		Username:     "",
		Mail:         "",
		MailVerified: false,
		DisplayName:  "",
		Groups:       nil,
		Opaque:       nil,
	}
	// ctx:=context.Background()

	var filters []*ocm.ListOCMSharesRequest_Filter
	share, err := managerWithData.listShares(&user, filters)
	if err != nil {
		t.Error(err)
	}
	if len(share) != 3 {
		t.Errorf("ListShares invalid list size got = %v, want %v", len(share), 3)
	}
}

func createManagerWithData() *mgr {
	now := time.Now().UnixNano()
	ts := &typespb.Timestamp{
		Seconds: uint64(now / 1000000000),
		Nanos:   uint32(now % 1000000000),
	}
	user := userpb.User{
		Id: &userpb.UserId{
			Idp:      "http://localhost:20080",
			OpaqueId: "4c510ada-c86b-4815-8820-42cdf82c3d51",
		},
	}
	g := &ocm.ShareGrant{
		Grantee:     nil,
		Permissions: &ocm.SharePermissions{},
	}

	s := &ocm.Share{
		Id: &ocm.ShareId{
			OpaqueId: "e45c5646-d202-4369-a21e-afe86985ea2a",
		},
		ResourceId: &provider.ResourceId{
			StorageId: "123e4567-e89b-12d3-a456-426655440000",
			OpaqueId:  "fileid-einstein/a.txt",
		},
		Permissions: g.Permissions,
		Grantee:     g.Grantee,
		Owner:       user.Id,
		Creator:     user.Id,
		Ctime:       ts,
		Mtime:       ts,
	}

	s2 := &ocm.Share{
		Id: &ocm.ShareId{
			OpaqueId: "1a28c96e-34b2-480c-b14b-8799b3f411f6",
		},
		ResourceId: &provider.ResourceId{
			StorageId: "123e4567-e89b-12d3-a456-426655440000",
			OpaqueId:  "fileid-einstein/b.txt",
		},
		Permissions: g.Permissions,
		Grantee:     g.Grantee,
		Owner:       user.Id,
		Creator:     user.Id,
		Ctime:       ts,
		Mtime:       ts,
	}

	s3 := &ocm.Share{
		Id: &ocm.ShareId{
			OpaqueId: "dd2ceead-852b-4d7c-8c89-73470c2a05ba",
		},
		ResourceId: &provider.ResourceId{
			StorageId: "123e4567-e89b-12d3-a456-426655440000",
			OpaqueId:  "fileid-einstein/c.txt",
		},
		Permissions: g.Permissions,
		Grantee:     g.Grantee,
		Owner:       user.Id,
		Creator:     user.Id,
		Ctime:       ts,
		Mtime:       ts,
	}

	m := &mgr{
		shares: sync.Map{},
		state:  nil,
	}

	storeShare(m, s)
	storeShare(m, s2)
	storeShare(m, s3)

	return m
}

func storeShare(m *mgr, s *ocm.Share) {

	key := &ocm.ShareKey{
		Owner:      s.Owner,
		ResourceId: s.ResourceId,
		Grantee:    s.Grantee,
	}

	m.shares.Store(key, s)
}
