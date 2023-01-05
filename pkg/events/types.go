// Copyright 2018-2023 CERN
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

package events

import (
	"encoding/json"

	group "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
)

// ShareCreated is emitted when a share is created.
type ShareCreated struct { // TODO: Rename to ShareCreatedEvent?
	Sharer *user.UserId
	// split the protobuf Grantee oneof so we can use stdlib encoding/json
	GranteeUserID  *user.UserId
	GranteeGroupID *group.GroupId
	Sharee         *provider.Grantee
	ItemID         *provider.ResourceId
	CTime          *types.Timestamp
}

// Unmarshal to fulfill umarshaller interface.
func (ShareCreated) Unmarshal(v []byte) (interface{}, error) {
	e := ShareCreated{}
	err := json.Unmarshal(v, &e)
	return e, err
}
