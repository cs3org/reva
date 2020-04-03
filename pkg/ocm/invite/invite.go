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

package invite

import (
	"context"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/invite/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
)

// Manager is the interface that is used to perform operations to invites.
type Manager interface {
	// GenerateToken creates a new token for the user with a specified validity.
	GenerateToken(ctx context.Context) (*invitepb.InviteToken, error)

	// ForwardInvite forwards a received invite to the sync'n'share system provider.
	ForwardInvite(ctx context.Context, invite *invitepb.InviteToken, originProvider *ocm.ProviderInfo) error

	// AcceptInvite completes an invitation acceptance.
	AcceptInvite(ctx context.Context, invite *invitepb.InviteToken, user *userpb.UserId, recipientProvider *ocm.ProviderInfo) error
}
