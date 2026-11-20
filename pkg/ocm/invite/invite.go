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

package invite

import (
	"context"
	"errors"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
)

// Repository is the interfaces used to store the tokens and the invited users.
//
// Callers must pass core-valid identities and secrets: every method validates
// its inputs (see validation.go) and returns errtypes.BadRequest before any
// storage access, so invalid input never writes or locks. Stored records are
// untrusted too: malformed rows are skipped or surfaced as internal errors
// instead of crashing lookups. Initiator keys are the unmodified opaque ids
// as provided; whether an empty Idp acts as a wildcard in GetRemoteUser and
// DeleteRemoteUser depends on the driver.
type Repository interface {
	// AddToken stores the token in the repository.
	AddToken(ctx context.Context, token *invitepb.InviteToken) error

	// GetToken gets the token from the repository.
	GetToken(ctx context.Context, token string) (*invitepb.InviteToken, error)

	// ListTokens lists core-valid tokens for the initiator, omitting elapsed expirations.
	// JSON and memory retain tokens without expiration; SQL does not store them.
	// Listing does not authorize redemption, which applies its own expiry check.
	ListTokens(ctx context.Context, initiator *userpb.UserId) ([]*invitepb.InviteToken, error)

	// AddRemoteUser stores the remote user.
	AddRemoteUser(ctx context.Context, initiator *userpb.UserId, remoteUser *userpb.User) error

	// GetRemoteUser retrieves details about a remote user who has accepted an invite to share.
	GetRemoteUser(ctx context.Context, initiator *userpb.UserId, remoteUserID *userpb.UserId) (*userpb.User, error)

	// FindRemoteUsers finds remote users who have accepted invites based on their attributes.
	FindRemoteUsers(ctx context.Context, initiator *userpb.UserId, query string) ([]*userpb.User, error)

	// DeleteRemoteUser removes from the remote user from the initiator's list.
	DeleteRemoteUser(ctx context.Context, initiator *userpb.UserId, remoteUser *userpb.UserId) error
}

// ErrTokenNotFound is the error returned when the token does not exist.
var ErrTokenNotFound = errors.New("token not found")

// ErrUserAlreadyAccepted is the error returned when the user was
// already added to the accepted users list.
var ErrUserAlreadyAccepted = errors.New("user already added to accepted users")
