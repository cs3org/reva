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
	"context"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

// ErrDataTransferStateUnavailable indicates that a directed decline cannot
// establish authoritative job state for a received DATATX projection.
var ErrDataTransferStateUnavailable = errtypes.Conflict("DATATX job state unavailable for directed decline")

// DirectedShareActions withdraws or rejects exact bound grants for an
// authenticated local creator or recipient.
// Historical grants without a complete binding remain available to ordinary
// per-share operations. Each direction changes its eligible selection atomically
// and returns receipts only after commit acknowledgment. CommitUnknownError
// leaves the outcome unknown. A received selection with a transfer in progress is refused.
// Stored DATATX access also fences the entire incoming action, in any projection
// state, because the action has no scheduler interlock. This can block decline
// for never-started or terminal jobs. Ordinary per-share operations still apply.
type DirectedShareActions interface {
	RemoveOCMSharesForRecipient(ctx context.Context, creator, recipient *userpb.UserId) ([]*ShareRemoval, error)
	RejectReceivedOCMSharesFromSender(ctx context.Context, recipient, sender *userpb.UserId) ([]*ReceivedShareUpdate, error)
}
