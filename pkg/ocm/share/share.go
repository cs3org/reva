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
	"context"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"google.golang.org/genproto/protobuf/field_mask"
)

// Repository is the interface that manipulates the OCM shares repository.
type Repository interface {
	// GenerateID generates a unique ID for a new share, so that the share can
	// be referenced (e.g. in the payload sent to the remote OCM server) before
	// it is persisted with StoreShare.
	GenerateID(ctx context.Context) (*ocm.ShareId, error)

	// StoreShare stores a share. The share must carry an ID pre-generated with
	// GenerateID, under which it is stored. The CS3 share has no resource
	// type, so it is stored separately and returned in the receipts.
	StoreShare(ctx context.Context, share *ocm.Share, resourceType ocm.SharedResourceType) (*ocm.Share, error)

	// GetShare gets the information for a share by the given ref.
	GetShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference) (*ocm.Share, error)

	// DeleteShare deletes the outgoing shares selected by ref and returns one
	// receipt per committed removal.
	DeleteShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference) ([]ShareRemoval, error)

	// UpdateShare updates the mode of the given share. The receipt compares
	// the stored values before and after the update.
	UpdateShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference, f ...*ocm.UpdateOCMShareRequest_UpdateField) (*ShareUpdate, error)

	// ListShares returns the shares created by the user. If md is provided is not nil,
	// it returns only shares attached to the given resource.
	ListShares(ctx context.Context, user *userpb.User, filters []*ocm.ListOCMSharesRequest_Filter) ([]*ocm.Share, error)

	// StoreReceivedShare stores a received share.
	StoreReceivedShare(ctx context.Context, share *ocm.ReceivedShare) (*ocm.ReceivedShare, error)

	// ListReceivedShares returns the list of shares the user has access.
	ListReceivedShares(ctx context.Context, user *userpb.User, filters []*ocm.ListReceivedOCMSharesRequest_Filter) ([]*ocm.ReceivedShare, error)

	// GetReceivedShare returns the information for a received share the user has access.
	GetReceivedShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference) (*ocm.ReceivedShare, error)

	// UpdateReceivedShare updates the received share with share state. A
	// state change invalidates any running embedded transfer attempt.
	UpdateReceivedShare(ctx context.Context, user *userpb.User, share *ocm.ReceivedShare, fieldMask *field_mask.FieldMask) (*ReceivedShareUpdate, error)

	// StartReceivedShareTransfer atomically moves a share that is neither
	// transferring nor accepted to TRANSFERRING and starts a new transfer attempt.
	StartReceivedShareTransfer(ctx context.Context, user *userpb.User, share *ocm.ReceivedShare, fieldMask *field_mask.FieldMask) (*TransferStart, error)

	// CompleteReceivedShareTransfer sets the final state of a transfer
	// attempt, only while that attempt is still running.
	CompleteReceivedShareTransfer(ctx context.Context, user *userpb.User, id *ocm.ShareId, generation uint64, state ocm.ShareState) (*ReceivedShareUpdate, error)

	// WithdrawReceivedGrant removes projections of the exact active grant and
	// their protocols. Drivers without atomic deletion return NotSupported.
	WithdrawReceivedGrant(ctx context.Context, w *GrantWithdrawal) (*GrantWithdrawalReceipt, error)

	// GetEmbeddedPayload returns the RO-Crate JSON payload carried by the embedded
	// protocol of a received share, or an empty string if the share carries no
	// embedded protocol. It is used to drive the background transfer of embedded
	// share contents.
	GetEmbeddedPayload(ctx context.Context, user *userpb.User, share *ocm.ReceivedShare) (string, error)
}

// ShareRemoval, ShareUpdate, ReceivedShareUpdate and TransferStart are only
// returned once their change is committed, with values read inside it.

// ShareRemoval is the receipt of the deletion of an outgoing share.
type ShareRemoval struct {
	Share *ocm.Share
	// ResourceType is the stored type. INVALID means it was unavailable.
	ResourceType ocm.SharedResourceType
}

// ShareUpdate is the receipt of an update of an outgoing share.
type ShareUpdate struct {
	Before *ocm.Share
	After  *ocm.Share
	// ResourceType is the stored type. INVALID means it was unavailable.
	ResourceType ocm.SharedResourceType
	// WebDAVPermissionsChanged and WebappPermissionsChanged compare the stored
	// permission values, not the presence of fields in the request.
	WebDAVPermissionsChanged bool
	WebappPermissionsChanged bool
}

// PermissionsChanged reports whether any stored permission value changed.
func (u *ShareUpdate) PermissionsChanged() bool {
	return u != nil && (u.WebDAVPermissionsChanged || u.WebappPermissionsChanged)
}

// ReceivedShareUpdate is the receipt of an update of a received share.
type ReceivedShareUpdate struct {
	Before *ocm.ReceivedShare
	After  *ocm.ReceivedShare
	// ClaimStateNotice is nil when the issuer does not provide proofs. A
	// nil result means this receipt cannot authorize a notice.
	ClaimStateNotice func() *StateNotice `json:"-"`
}

// StateNotice belongs to one claimant. The issuer can retire it at any time.
type StateNotice struct {
	StillCurrent func() bool
	Release      func()
}

type stateNoticeKey struct{}

// WithStateNotice requests a proof for a received-state mutation that may notify.
func WithStateNotice(ctx context.Context) context.Context {
	return context.WithValue(ctx, stateNoticeKey{}, true)
}

// WantsStateNotice reports whether the caller requested a notice proof.
func WantsStateNotice(ctx context.Context) bool {
	return ctx.Value(stateNoticeKey{}) == true
}

// StateChanged reports whether the persisted state changed.
func (u *ReceivedShareUpdate) StateChanged() bool {
	return u != nil && u.Before != nil && u.After != nil && u.Before.State != u.After.State
}

// TransferStart is the receipt of an embedded transfer start.
type TransferStart struct {
	Share *ocm.ReceivedShare
	// Generation identifies this attempt; only its completion is accepted.
	Generation uint64
}

// ErrInvalidResourceType is returned when creating a share without a supported
// type or when an existing share has an unsupported stored type.
var ErrInvalidResourceType = errtypes.InternalError("share has no supported resource type")

// SupportedResourceType reports whether an outgoing share can be stored with t.
func SupportedResourceType(t ocm.SharedResourceType) bool {
	switch t {
	case ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE,
		ocm.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER,
		ocm.SharedResourceType_SHARE_RESOURCE_TYPE_EMBEDDED:
		return true
	}
	return false
}

// ErrTransferInProgress is returned when a transfer is already running.
var ErrTransferInProgress = errtypes.Conflict("transfer already in progress")

// ErrShareAlreadyAccepted is returned when a transfer starts for a share that
// is accepted already. Set another state first to copy the share again.
var ErrShareAlreadyAccepted = errtypes.Conflict("share already accepted")

// ErrTransferGenerationExhausted is returned instead of wrapping the
// transfer attempt counter.
var ErrTransferGenerationExhausted = errtypes.InternalError("transfer attempt counter exhausted")

// ResourceIDFilter is an abstraction for creating filter by resource id.
func ResourceIDFilter(id *provider.ResourceId) *ocm.ListOCMSharesRequest_Filter {
	return &ocm.ListOCMSharesRequest_Filter{
		Type: ocm.ListOCMSharesRequest_Filter_TYPE_RESOURCE_ID,
		Term: &ocm.ListOCMSharesRequest_Filter_ResourceId{
			ResourceId: id,
		},
	}
}

// ErrShareAlreadyExisting is the error returned when the share already exists
// for the 3-tuple consisting of (owner, resource, grantee).
var ErrShareAlreadyExisting = errtypes.AlreadyExists("share already exists")

// ErrShareNotFound is the error returned where the share does not exist.
var ErrShareNotFound = errtypes.NotFound("share not found")
