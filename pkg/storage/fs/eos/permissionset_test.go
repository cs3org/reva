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

package eos

import (
	"context"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/cs3org/reva/v3/pkg/storage/utils/acl"
)

// EOS applies sys.mask before it checks the mode of the parent folder, so a
// file in a 0644 folder under the usual 700 mask is not world readable.
func TestPermissionSetAppliesSysMask(t *testing.T) {
	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{OpaqueId: "other", Type: userpb.UserType_USER_TYPE_PRIMARY},
	})
	owner := &userpb.UserId{OpaqueId: "owner", Type: userpb.UserType_USER_TYPE_PRIMARY}

	tests := []struct {
		name     string
		mode     uint64
		attrs    map[string]string
		wantRead bool
	}{
		{"no mask", 0o644, map[string]string{}, true},
		{"mask hides world read", 0o644, map[string]string{"sys.mask": "700"}, false},
		{"mask keeps world read", 0o644, map[string]string{"sys.mask": "755"}, true},
		{"empty mask", 0o644, map[string]string{"sys.mask": ""}, true},
		{"no world read", 0o640, map[string]string{}, false},
	}

	fs := &Eosfs{}
	for _, tt := range tests {
		got := fs.permissionSet(ctx, &eosclient.FileInfo{ParentMode: tt.mode, Attrs: tt.attrs}, owner)
		if got.InitiateFileDownload != tt.wantRead {
			t.Errorf("%s: InitiateFileDownload = %v, want %v", tt.name, got.InitiateFileDownload, tt.wantRead)
		}
	}
}

// EOS does not check the mode of a file, only the mode of its parent folder.
func TestPermissionSetFileUsesParentMode(t *testing.T) {
	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{OpaqueId: "other", Type: userpb.UserType_USER_TYPE_PRIMARY},
	})
	owner := &userpb.UserId{OpaqueId: "owner", Type: userpb.UserType_USER_TYPE_PRIMARY}

	tests := []struct {
		name       string
		mode       uint64
		parentMode uint64
		wantRead   bool
	}{
		{"world-readable file in private folder", 0o644, 0o2700, false},
		{"private file in world-readable folder", 0o600, 0o2755, true},
		{"parent unknown", 0o644, 0, false},
	}

	fs := &Eosfs{}
	for _, tt := range tests {
		info := &eosclient.FileInfo{Mode: tt.mode, ParentMode: tt.parentMode, Attrs: map[string]string{}}
		got := fs.permissionSet(ctx, info, owner)
		if got.InitiateFileDownload != tt.wantRead {
			t.Errorf("%s: InitiateFileDownload = %v, want %v", tt.name, got.InitiateFileDownload, tt.wantRead)
		}
	}
}

// A lightweight account must not read a world-readable resource without a share.
func TestPermissionSetLightweightIgnoresWorldRead(t *testing.T) {
	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{OpaqueId: "lw@example.org", Type: userpb.UserType_USER_TYPE_LIGHTWEIGHT},
	})
	owner := &userpb.UserId{OpaqueId: "owner", Type: userpb.UserType_USER_TYPE_PRIMARY}

	tests := []struct {
		name     string
		attrs    map[string]string
		wantRead bool
	}{
		{"no share", map[string]string{}, false},
		{"share", map[string]string{"sys.reva.lwshare.lw@example.org": "rx"}, true},
		{"share for another account", map[string]string{"sys.reva.lwshare.other@example.org": "rx"}, false},
	}

	fs := &Eosfs{}
	for _, tt := range tests {
		info := &eosclient.FileInfo{Mode: 0o755, IsDir: true, Attrs: tt.attrs, SysACL: &acl.ACLs{}}
		got := fs.permissionSet(ctx, info, owner)
		if got.InitiateFileDownload != tt.wantRead {
			t.Errorf("%s: InitiateFileDownload = %v, want %v", tt.name, got.InitiateFileDownload, tt.wantRead)
		}
	}
}
