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

package sql

import (
	"cmp"
	"context"
	"slices"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/providerdomain"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

const directedObservationBatchSize = 500

// RemoveOCMSharesForRecipient uses the bound creator identity, including tenant,
// because a resource owner or a same-opaque principal may be a different user.
func (m *mgr) RemoveOCMSharesForRecipient(ctx context.Context, creator, recipient *userpb.UserId) ([]*share.ShareRemoval, error) {
	if creator.GetOpaqueId() == "" || creator.GetIdp() == "" || creator.GetType() == userpb.UserType_USER_TYPE_INVALID || recipient.GetOpaqueId() == "" {
		return nil, errtypes.BadRequest("invalid directed share action identity")
	}
	remoteHost, err := providerdomain.CanonicalOCMHost(recipient.GetIdp())
	if err != nil {
		return nil, errtypes.BadRequest("invalid recipient provider host")
	}
	var receipts []*share.ShareRemoval
	var check commitCheck
	err = m.transact(ctx, "remove_shares_for_recipient", func(tx *gorm.DB) error {
		receipts = nil
		check = commitCheck{}
		q := forUpdate(tx).
			Where("creator_idp = ? AND remote_recipient = ? AND initiator = ? AND creator_user_type = ? AND creator_tenant = ? AND recipient_type = ?",
				[]byte(creator.GetIdp()), []byte(recipient.GetOpaqueId()+"@"+remoteHost), creator.GetOpaqueId(), int32(creator.GetType()), []byte(creator.GetTenantId()), model.OcmShareTypeUser).
			Order(outgoingShareOrder)
		var selected []model.OcmShare
		if err := q.Find(&selected).Error; err != nil {
			return err
		}
		authorized := selected[:0]
		for _, sm := range selected {
			// MySQL may collate the ordinary initiator column non-binarily.
			if sm.Initiator != creator.GetOpaqueId() {
				continue
			}
			if !share.SupportedResourceType(convertToCS3SharedResourceType(sm.ItemType)) {
				return share.ErrInvalidResourceType
			}
			authorized = append(authorized, sm)
		}
		if len(authorized) == 0 {
			return nil
		}
		if err := deleteLockedOCMShares(tx, authorized, errtypes.Conflict("selected share changed")); err != nil {
			return err
		}

		// Preserve ID order in the receipt after using database order to delete.
		slices.SortFunc(authorized, func(a, b model.OcmShare) int { return cmp.Compare(a.Id, b.Id) })
		ids := make([]uint, 0, len(authorized))
		for i := range authorized {
			sm := &authorized[i]
			methods, err := accessMethodsTx(tx, sm.Id)
			if err != nil {
				return err
			}
			ids = append(ids, sm.Id)
			receipts = append(receipts, &share.ShareRemoval{Share: convertToCS3OCMShare(sm, methods), ResourceType: convertToCS3SharedResourceType(sm.ItemType)})
		}
		check.observe = func(db *gorm.DB) (share.CommitObservation, error) {
			var active int64
			err := observeTx(db, func(tx *gorm.DB) error {
				for chunk := range slices.Chunk(ids, directedObservationBatchSize) {
					var count int64
					if err := tx.Model(&model.OcmShare{}).Where("id IN ?", chunk).Count(&count).Error; err != nil {
						return err
					}
					active += count
				}
				return nil
			})
			if err != nil {
				return share.CommitObservedUnread, err
			}
			if active == 0 {
				return share.CommitObservedRequested, nil
			}
			return share.CommitObservedOther, nil
		}
		return nil
	}, &check)
	if err != nil {
		return nil, err
	}
	return receipts, nil
}

// RejectReceivedOCMSharesFromSender applies local recipient intent to active projections.
func (m *mgr) RejectReceivedOCMSharesFromSender(ctx context.Context, recipient, sender *userpb.UserId) ([]*share.ReceivedShareUpdate, error) {
	if recipient.GetOpaqueId() == "" || recipient.GetIdp() == "" || recipient.GetType() == userpb.UserType_USER_TYPE_INVALID || sender.GetOpaqueId() == "" {
		return nil, errtypes.BadRequest("invalid directed share action identity")
	}
	senderHost, err := share.CanonicalGrantOrigin(sender.GetIdp())
	if err != nil {
		return nil, errtypes.BadRequest("invalid sender provider host")
	}
	var receipts []*share.ReceivedShareUpdate
	var check commitCheck
	err = m.transact(ctx, "reject_received_from_sender", func(tx *gorm.DB) error {
		receipts = nil
		check = commitCheck{}
		var selected []model.OcmReceivedShare
		senderAddress := sender.GetOpaqueId() + "@" + senderHost
		activeStates := []model.OcmShareState{model.OcmShareStatePending, model.OcmShareStateAccepted, model.OcmShareStateTransferring}
		if err := forUpdate(tx).
			Where("initiator = ? AND share_with = ? AND share_with_idp = ? AND share_with_tenant = ? AND recipient_user_type = ? AND recipient_type = ?",
				senderAddress, recipient.GetOpaqueId(), recipient.GetIdp(), recipient.GetTenantId(), int32(recipient.GetType()), model.OcmShareTypeUser).
			Where("(state IN ? OR grant_key IS NOT NULL)", activeStates).
			Order("id").Find(&selected).Error; err != nil {
			return err
		}
		authorized := selected[:0]
		transferring := false
		for _, row := range selected {
			if row.Initiator != senderAddress || row.ShareWith != recipient.GetOpaqueId() ||
				row.ShareWithIdp != recipient.GetIdp() || row.ShareWithTenant != recipient.GetTenantId() {
				// The inherited text columns may match under a looser collation.
				continue
			}
			active := slices.Contains(activeStates, row.State)
			key, err := share.GrantKey(senderHost, row.RemoteShareID)
			if err != nil || row.GrantKey == nil || *row.GrantKey != key {
				if active {
					return share.ErrGrantIdentityMismatch
				}
				continue
			}
			if !share.SupportedResourceType(convertToCS3SharedResourceType(row.ItemType)) {
				if active {
					return share.ErrInvalidResourceType
				}
				continue
			}
			authorized = append(authorized, row)
			transferring = transferring || row.State == model.OcmShareStateTransferring
		}
		// Keep the transfer state while the copy runs so a second accept
		// cannot start another copy.
		if transferring {
			return share.ErrTransferInProgress
		}
		type eligibleProjection struct {
			row    model.OcmReceivedShare
			before *ocm.ReceivedShare
		}
		var eligible []eligibleProjection
		for _, row := range authorized {
			protocols, err := protocolsTx(tx, row.ID)
			if err != nil {
				return err
			}
			before := convertToCS3OCMReceivedShare(&row, protocols)
			// Ordinary state changes do not fence the scheduler.
			if share.DataTransferProtocol(before) != nil {
				return share.ErrDataTransferStateUnavailable
			}
			if slices.Contains(activeStates, row.State) {
				eligible = append(eligible, eligibleProjection{row: row, before: before})
			}
		}
		if len(eligible) == 0 {
			return nil
		}

		type stateSnapshot struct {
			State              model.OcmShareState
			Hidden             bool
			TransferGeneration uint64
		}
		ids := make([]uint, 0, len(eligible))
		previous := make([]stateSnapshot, 0, len(eligible))
		expected := make([]stateSnapshot, 0, len(eligible))
		for _, projection := range eligible {
			row := projection.row
			previous = append(previous, stateSnapshot{row.State, row.Hidden, row.TransferGeneration})
			if err := updateReceivedFields(tx, &row, map[string]any{
				"state": model.OcmShareStateRejected, "hidden": true, "mtime": time.Now().Unix(),
			}); err != nil {
				return err
			}
			var applied model.OcmReceivedShare
			result := tx.Find(&applied, row.ID)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return share.ErrShareNotFound
			}
			protocols := slices.Clone(projection.before.Protocols)
			for i, protocol := range protocols {
				protocols[i] = proto.CloneOf(protocol)
			}
			after := convertToCS3OCMReceivedShare(&applied, protocols)
			ids = append(ids, row.ID)
			expected = append(expected, stateSnapshot{applied.State, applied.Hidden, applied.TransferGeneration})
			receipts = append(receipts, &share.ReceivedShareUpdate{Before: projection.before, After: after})
		}
		check.observe = func(db *gorm.DB) (share.CommitObservation, error) {
			var observed []stateSnapshot
			err := observeTx(db, func(tx *gorm.DB) error {
				// The captured IDs are already in ascending order.
				for chunk := range slices.Chunk(ids, directedObservationBatchSize) {
					var states []stateSnapshot
					err := tx.Model(&model.OcmReceivedShare{}).Select("state", "hidden", "transfer_generation").
						Where("id IN ?", chunk).Order("id").Find(&states).Error
					if err != nil {
						return err
					}
					observed = append(observed, states...)
				}
				return nil
			})
			if err != nil {
				return share.CommitObservedUnread, err
			}
			switch {
			case slices.Equal(observed, expected):
				return share.CommitObservedRequested, nil
			case slices.Equal(observed, previous):
				return share.CommitObservedPrevious, nil
			default:
				return share.CommitObservedOther, nil
			}
		}
		return nil
	}, &check)
	if err != nil {
		return nil, err
	}
	return receipts, nil
}
