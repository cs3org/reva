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

package invite

import (
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

// ValidateUserID rejects a nil id or an id whose opaque part is blank. It
// returns a raw errtypes.BadRequest so status helpers can switch on the
// concrete type. Idp and user type stay optional: identities are stored and
// compared exactly as provided, without rewriting them.
func ValidateUserID(id *userpb.UserId, field string) error {
	if id == nil {
		return errtypes.BadRequest("missing " + field)
	}
	if strings.TrimSpace(id.OpaqueId) == "" {
		return errtypes.BadRequest("blank " + field)
	}
	return nil
}

// ValidateRemoteUser rejects a nil remote user and then requires a valid id.
func ValidateRemoteUser(user *userpb.User) error {
	if user == nil {
		return errtypes.BadRequest("missing remote user")
	}
	return ValidateUserID(user.Id, "remote user id")
}

// ValidateInviteToken requires the token secret and the issuing user id. The
// CS3 expiration field is optional and deliberately not required here;
// callers that depend on an expiry check it separately.
func ValidateInviteToken(token *invitepb.InviteToken) error {
	if token == nil {
		return errtypes.BadRequest("missing invite token")
	}
	if strings.TrimSpace(token.Token) == "" {
		return errtypes.BadRequest("blank invite token")
	}
	return ValidateUserID(token.UserId, "invite token user id")
}
