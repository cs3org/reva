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
)

// DirectedShareActions is the transactional bulk surface of a repository:
// withdraw exact bound outgoing grants and reject exact bound received grants.
// Each direction either commits its whole captured selection or reports an
// error without an acknowledged receipt.
type DirectedShareActions interface {
	RemoveOCMSharesForRecipient(ctx context.Context, creator, recipient *userpb.UserId) ([]*ShareRemoval, error)
	RejectReceivedOCMSharesFromSender(ctx context.Context, recipient, sender *userpb.UserId) ([]*ReceivedShareUpdate, error)
}
