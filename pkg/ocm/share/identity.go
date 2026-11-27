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

package share

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/providerdomain"
)

// MaxGrantFieldLength bounds the origin and remote share ID of a grant
// identity, in bytes.
const MaxGrantFieldLength = 255

// ErrInvalidGrantIdentity is returned for an origin or remote share ID that
// cannot form a grant identity.
var ErrInvalidGrantIdentity = errtypes.BadRequest("invalid grant identity")

// ErrCommitUnknown is returned when a change was sent for commit but the
// commit was not acknowledged. The change may be durable; it has no receipt.
var ErrCommitUnknown = errtypes.InternalError("the outcome of the change is unknown")

// CommitObservation is the durable state read after an unacknowledged
// commit. A state equal to the requested one does not prove that this
// caller made the change.
type CommitObservation string

const (
	CommitObservedRequested CommitObservation = "requested_state"
	CommitObservedPrevious  CommitObservation = "previous_state"
	CommitObservedOther     CommitObservation = "other_state"
	CommitObservedUnread    CommitObservation = "unread"
)

// CommitUnknownError is an ErrCommitUnknown with the state read afterwards.
type CommitUnknownError struct {
	Observed CommitObservation
}

func (e *CommitUnknownError) Error() string {
	return ErrCommitUnknown.Error() + " (" + string(e.Observed) + ")"
}

func (e *CommitUnknownError) Is(target error) bool { return target == ErrCommitUnknown }

// ErrGrantIdentityMismatch is returned when a stored projection does
// not hold exactly the fields its key was computed from. It fails closed.
var ErrGrantIdentityMismatch = errtypes.InternalError("grant projection does not match its key")

// CanonicalGrantOrigin validates the host[:port] of the provider that sent a
// share with the OCM address host grammar and lowercases it. A scheme, path
// or any other invalid part is rejected, never stripped.
func CanonicalGrantOrigin(domain string) (string, error) {
	if len(domain) > MaxGrantFieldLength {
		return "", ErrInvalidGrantIdentity
	}
	origin, err := providerdomain.CanonicalOCMHost(domain)
	if err != nil {
		return "", ErrInvalidGrantIdentity
	}
	return origin, nil
}

// ReceivedGrantOrigin validates the creator of a new received share and
// returns its origin. The opaque ID and the separate host are checked before
// they are joined, so an @ in the host cannot move the origin.
func ReceivedGrantOrigin(creator *userpb.UserId) (string, error) {
	if creator.GetOpaqueId() == "" {
		return "", ErrInvalidGrantIdentity
	}
	return CanonicalGrantOrigin(creator.GetIdp())
}

// GrantKey returns the identity key of a provider grant, a length-prefixed
// SHA-256 of the canonical origin and the byte-exact remote share ID.
func GrantKey(origin, remoteShareID string) (string, error) {
	if origin == "" || remoteShareID == "" || len(origin) > MaxGrantFieldLength || len(remoteShareID) > MaxGrantFieldLength {
		return "", ErrInvalidGrantIdentity
	}
	h := sha256.New()
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(origin)))
	h.Write(n[:])
	h.Write([]byte(origin))
	binary.BigEndian.PutUint32(n[:], uint32(len(remoteShareID)))
	h.Write(n[:])
	h.Write([]byte(remoteShareID))
	return hex.EncodeToString(h.Sum(nil)), nil
}

// GrantWithdrawal identifies an active provider grant and the types its
// stored shares must match.
type GrantWithdrawal struct {
	Origin        string
	RemoteShareID string
	ResourceType  ocm.SharedResourceType
	// RecipientType is optional; when set the stored shares must match it.
	RecipientType ocm.RecipientType
}

// GrantWithdrawalReceipt reports whether the transaction removed a projection.
type GrantWithdrawalReceipt struct {
	Changed bool
}
