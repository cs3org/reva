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

package eos

import (
	"testing"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
)

// the eos lock attr only carries the expiration, so the creation time only reaches
// PROPFIND if it survives being stored in the payload attr
func TestLockCreationTimeSurvivesTheLockPayload(t *testing.T) {
	fs := &Eosfs{}
	lock := &provider.Lock{
		LockId:       "some-id",
		Type:         provider.LockType_LOCK_TYPE_WRITE,
		AppName:      "MicrosoftOffice",
		Expiration:   &types.Timestamp{Seconds: uint64(time.Now().Add(time.Hour).Unix())},
		CreationTime: &types.Timestamp{Seconds: 1786432355},
	}

	payload, eosLock, err := fs.encodeLock(lock)
	if err != nil {
		t.Fatalf("failed to encode the lock: %v", err)
	}
	decoded, err := decodeLock(payload, eosLock)
	if err != nil {
		t.Fatalf("failed to decode the lock: %v", err)
	}

	if got := decoded.GetCreationTime().GetSeconds(); got != 1786432355 {
		t.Fatalf("expected the creation time to survive the round trip, got %d", got)
	}
}
