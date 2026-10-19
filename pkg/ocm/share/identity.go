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

package share

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"unicode"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

// MaxGrantFieldLength bounds the origin and remote share ID of a grant
// identity, in bytes.
const MaxGrantFieldLength = 255

// WithdrawnShareMessage is the fixed status message of a mutation refused
// because the provider withdrew the share.
const WithdrawnShareMessage = "the share was withdrawn by its provider"

// ErrShareWithdrawn is returned when a writer tries to change or recreate a
// received share whose provider grant was withdrawn.
var ErrShareWithdrawn = errtypes.Conflict(WithdrawnShareMessage)

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

// ErrGrantIdentityMismatch is returned when a stored identity record does
// not hold exactly the fields its key was computed from. It fails closed.
var ErrGrantIdentityMismatch = errtypes.InternalError("grant identity record does not match its key")

// CanonicalGrantOrigin lowercases the domain of the provider that sent a
// share and drops an http(s) scheme. Anything but a plain host[:port] is rejected.
func CanonicalGrantOrigin(domain string) (string, error) {
	d := strings.TrimPrefix(strings.TrimPrefix(domain, "https://"), "http://")
	d = strings.ToLower(d)
	if d == "" || len(d) > MaxGrantFieldLength {
		return "", ErrInvalidGrantIdentity
	}
	for _, r := range d {
		if r > unicode.MaxASCII || unicode.IsSpace(r) || unicode.IsControl(r) || strings.ContainsRune("/@?#\\", r) {
			return "", ErrInvalidGrantIdentity
		}
	}
	return d, nil
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

// GrantWithdrawal identifies a provider grant to withdraw and the types its
// stored shares must match.
type GrantWithdrawal struct {
	Origin        string
	RemoteShareID string
	ResourceType  ocm.SharedResourceType
	// RecipientType is optional; when set the stored shares must match it.
	RecipientType ocm.RecipientType
}

// GrantWithdrawalReceipt is the committed result of a withdrawal.
type GrantWithdrawalReceipt struct {
	// Changed is true only for the call that set the withdrawal.
	Changed bool
	// Shares are the retained received shares of the grant after the call.
	Shares []*ocm.ReceivedShare
}
