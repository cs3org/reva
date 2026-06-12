// Copyright 2018-2021 CERN
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

package storagespace

import (
	"context"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"

	"github.com/owncloud/reva/v2/pkg/storage"
)

type key int

const (
	spaceOwnerSlotKey key = iota
	moveResultSlotKey key = iota
)

// --- space owner ---

type spaceOwnerSlot struct {
	ownerID *userpb.UserId
}

// ContextRegisterSpaceOwnerSlot installs an empty slot in ctx so that the
// storage driver can write the space owner via ContextSetSpaceOwner and the
// events middleware can read it via ContextGetSpaceOwner after the handler
// returns. Subsequent registrations are no-ops; the first registration wins.
func ContextRegisterSpaceOwnerSlot(ctx context.Context) context.Context {
	if ctx.Value(spaceOwnerSlotKey) != nil {
		return ctx
	}
	return context.WithValue(ctx, spaceOwnerSlotKey, &spaceOwnerSlot{})
}

// ContextSetSpaceOwner writes id into the slot. Subsequent writes overwrite
// the previous value. Does nothing if no slot was registered.
func ContextSetSpaceOwner(ctx context.Context, id *userpb.UserId) {
	if slot, ok := ctx.Value(spaceOwnerSlotKey).(*spaceOwnerSlot); ok {
		slot.ownerID = id
	}
}

// ContextGetSpaceOwner returns the space owner written by the storage driver,
// or nil if none was set.
func ContextGetSpaceOwner(ctx context.Context) *userpb.UserId {
	if slot, ok := ctx.Value(spaceOwnerSlotKey).(*spaceOwnerSlot); ok {
		return slot.ownerID
	}
	return nil
}

// --- move result ---

type moveResultSlot struct {
	result *storage.MoveResult
}

// ContextRegisterMoveResultSlot installs an empty slot in ctx so that the
// storage driver can write move metadata via ContextSetMoveResult and the
// events middleware can read it via ContextGetMoveResult after the handler
// returns. Subsequent registrations are no-ops; the first registration wins.
func ContextRegisterMoveResultSlot(ctx context.Context) context.Context {
	if ctx.Value(moveResultSlotKey) != nil {
		return ctx
	}
	return context.WithValue(ctx, moveResultSlotKey, &moveResultSlot{})
}

// ContextSetMoveResult writes r into the slot. Subsequent writes overwrite
// the previous value. Does nothing if no slot was registered.
func ContextSetMoveResult(ctx context.Context, r *storage.MoveResult) {
	if slot, ok := ctx.Value(moveResultSlotKey).(*moveResultSlot); ok {
		slot.result = r
	}
}

// ContextGetMoveResult returns the move result written by the storage driver,
// or nil if none was set.
func ContextGetMoveResult(ctx context.Context) *storage.MoveResult {
	if slot, ok := ctx.Value(moveResultSlotKey).(*moveResultSlot); ok {
		return slot.result
	}
	return nil
}
