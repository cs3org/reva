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
)

// EOS applies sys.mask before it checks the mode, so a 0644 file under the
// usual 700 mask is not world readable.
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
		got := fs.permissionSet(ctx, &eosclient.FileInfo{Mode: tt.mode, Attrs: tt.attrs}, owner)
		if got.InitiateFileDownload != tt.wantRead {
			t.Errorf("%s: InitiateFileDownload = %v, want %v", tt.name, got.InitiateFileDownload, tt.wantRead)
		}
	}
}
