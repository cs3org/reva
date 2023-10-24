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

package spaces

import (
	"context"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// Manager is the interface that stores the spaces.
type Manager interface {
	StoreSpace(ctx context.Context, owner *userpb.UserId, path, name string, quota *provider.Quota) error
	ListSpaces(ctx context.Context, user *userpb.User) ([]*provider.StorageSpace, error)
	UpdateSpace(ctx context.Context, space *provider.StorageSpace) error
	DeleteSpace(ctx context.Context, spaceID *provider.StorageSpaceId) error
}
