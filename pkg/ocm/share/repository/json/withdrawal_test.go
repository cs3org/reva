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

package json

import (
	"context"
	"errors"
	"testing"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
)

// The JSON driver records no withdrawal and changes nothing.
func TestJSONWithdrawalIsNotSupported(t *testing.T) {
	m, cleanup := setup(t)
	defer cleanup()
	user, rs := receivedFixture(t, m)
	r, err := m.WithdrawReceivedGrant(context.Background(), &share.GrantWithdrawal{
		Origin: "provider.example.org", RemoteShareID: rs.RemoteShareId, ResourceType: rs.SharedResourceType,
	})
	var notSupported errtypes.NotSupported
	if r != nil || !errors.As(err, &notSupported) {
		t.Fatalf("withdraw: %v %v", r, err)
	}
	got, err := m.GetReceivedShare(context.Background(), user, &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}})
	if err != nil || got.State != ocm.ShareState_SHARE_STATE_PENDING || got.Hidden {
		t.Fatalf("share changed: %v %v", got, err)
	}
}
