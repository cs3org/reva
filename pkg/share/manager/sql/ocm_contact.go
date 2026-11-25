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

// Directed bulk share actions. Each direction captures its exact selection
// inside one transaction and either commits all of it or returns without an
// acknowledged receipt. Grants created outside the captured set are later
// separate shares; no pair-wide authority is recorded here.

import (
	"context"
	"slices"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/providerdomain"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	model "github.com/cs3org/reva/v3/pkg/share/manager/sql/model"
	"gorm.io/gorm"
)

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
			Order("id")
		var selected []model.OcmShare
		if err := q.Find(&selected).Error; err != nil {
			return err
		}
		if len(selected) == 0 {
			return nil
		}
		ids := make([]uint, 0, len(selected))
		authorized := make([]model.OcmShare, 0, len(selected))
		for _, sm := range selected {
			// MySQL may collate the ordinary initiator column non-binarily.
			if sm.Initiator != creator.GetOpaqueId() {
				continue
			}
			if !share.SupportedResourceType(convertToCS3SharedResourceType(sm.ItemType)) {
				return share.ErrInvalidResourceType
			}
			ids = append(ids, sm.Id)
			authorized = append(authorized, sm)
			methods, err := accessMethodsTx(tx, sm.Id)
			if err != nil {
				return err
			}
			receipts = append(receipts, &share.ShareRemoval{Share: convertToCS3OCMShare(&sm, methods), ResourceType: convertToCS3SharedResourceType(sm.ItemType)})
		}
		if len(ids) == 0 {
			return nil
		}
		if err := deleteLockedOCMShares(tx, authorized, errtypes.Conflict("selected share changed")); err != nil {
			return err
		}
		check.observe = func(db *gorm.DB) (share.CommitObservation, error) {
			var active int64
			if err := db.Model(&model.OcmShare{}).Where("id IN ?", ids).Count(&active).Error; err != nil {
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
		if err := forUpdate(tx).
			Where("initiator = ? AND share_with = ? AND share_with_idp = ? AND share_with_tenant = ? AND recipient_user_type = ? AND recipient_type = ?",
				senderAddress, recipient.GetOpaqueId(), recipient.GetIdp(), recipient.GetTenantId(), int32(recipient.GetType()), model.OcmShareTypeUser).
			Where("state IN ?", []model.OcmShareState{model.OcmShareStatePending, model.OcmShareStateAccepted, model.OcmShareStateTransferring}).
			Order("id").Find(&selected).Error; err != nil {
			return err
		}
		if len(selected) == 0 {
			return nil
		}
		type stateSnapshot struct {
			State              model.OcmShareState
			Hidden             bool
			TransferGeneration uint64
		}
		ids := make([]uint, 0, len(selected))
		previous := make([]stateSnapshot, 0, len(selected))
		expected := make([]stateSnapshot, 0, len(selected))
		for _, row := range selected {
			if row.Initiator != senderAddress || row.ShareWith != recipient.GetOpaqueId() ||
				row.ShareWithIdp != recipient.GetIdp() || row.ShareWithTenant != recipient.GetTenantId() {
				// The inherited text columns may match under a looser collation.
				continue
			}
			key, err := share.GrantKey(senderHost, row.RemoteShareID)
			if err != nil || row.GrantKey == nil || *row.GrantKey != key {
				return share.ErrGrantIdentityMismatch
			}
			if !share.SupportedResourceType(convertToCS3SharedResourceType(row.ItemType)) {
				return share.ErrInvalidResourceType
			}
			protocols, err := protocolsTx(tx, row.ID)
			if err != nil {
				return err
			}
			before := convertToCS3OCMReceivedShare(&row, protocols)
			previous = append(previous, stateSnapshot{row.State, row.Hidden, row.TransferGeneration})
			if err := updateReceivedFields(tx, &row, map[string]any{
				"state": model.OcmShareStateRejected, "hidden": true, "mtime": time.Now().Unix(),
			}); err != nil {
				return err
			}
			applied, err := readReceivedProjection(tx, row.ID)
			if err != nil {
				return err
			}
			if applied == nil {
				return share.ErrShareNotFound
			}
			var appliedProtocols []*ocm.Protocol
			for i := range applied.protocols {
				appliedProtocols = append(appliedProtocols, convertToCS3Protocol(&applied.protocols[i]))
			}
			after := convertToCS3OCMReceivedShare(&applied.row, appliedProtocols)
			ids = append(ids, row.ID)
			expected = append(expected, stateSnapshot{applied.row.State, applied.row.Hidden, applied.row.TransferGeneration})
			receipts = append(receipts, &share.ReceivedShareUpdate{Before: before, After: after})
		}
		if len(ids) == 0 {
			return nil
		}
		check.observe = func(db *gorm.DB) (share.CommitObservation, error) {
			var observed []stateSnapshot
			err := db.Model(&model.OcmReceivedShare{}).Select("state", "hidden", "transfer_generation").
				Where("id IN ?", ids).Order("id").Find(&observed).Error
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
