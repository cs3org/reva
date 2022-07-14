// Copyright 2018-2022 CERN
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

package action

import (
	"fmt"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/sdk"
	"github.com/cs3org/reva/pkg/sdk/common/net"
)

// RecycleOperationsAction offers recycle bin operations.
type RecycleOperationsAction struct {
	action
}

// Purge purges the entire recycle bin of the current user.
func (action *RecycleOperationsAction) Purge() error {
	// Get the home directory to purge the entire recycle bin
	fileOpsAct := MustNewFileOperationsAction(action.session)
	homePath, err := fileOpsAct.GetHome()
	if err != nil {
		return err
	}

	// Send purge request
	ref := &provider.Reference{Path: homePath}
	req := &provider.PurgeRecycleRequest{Ref: ref}
	res, err := action.session.Client().PurgeRecycle(action.session.Context(), req)
	if err := net.CheckRPCInvocation("purging recycle bin", res, err); err != nil {
		return err
	}
	return nil
}

// NewRecycleOperationsAction creates a new recycle operations action.
func NewRecycleOperationsAction(session *sdk.Session) (*RecycleOperationsAction, error) {
	action := &RecycleOperationsAction{}
	if err := action.initAction(session); err != nil {
		return nil, fmt.Errorf("unable to create the RecycleOperationsAction: %v", err)
	}
	return action, nil
}

// MustNewRecycleOperationsAction creates a new recycle operations action and panics on failure.
func MustNewRecycleOperationsAction(session *sdk.Session) *RecycleOperationsAction {
	action, err := NewRecycleOperationsAction(session)
	if err != nil {
		panic(err)
	}
	return action
}
