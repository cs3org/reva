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

package storage

import (
	"context"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
)

// InMemoryFS is supposed to be a storage.FS implementation which can be used in unit tests which depend on a storage.
// Not all methods are implemented but can be added when required.
// The data is only stored in memory.
type InMemoryFS struct {
	FS
	// Resources acts as the in memory storage.
	// The expected layout is as follows:
	//
	// Resources: map[string]map[string]*provider.ResourceInfo{
	// 	userOne.Id.OpaqueId: {
	// 		resourceInfoOne.Id.OpaqueId: resourceInfoOne,
	// 	},
	// 	userTwo.Id.OpaqueId: {
	// 		resourceInfoOne.Id.OpaqueId: resourceInfoOne,
	// 		resourceInfoTwo.Id.OpaqueId: resourceInfoTwo,
	// 	},
	// }
	Resources map[string]map[string]*provider.ResourceInfo
}

// GetMD looks up the ResourceInfo by a Reference
func (fs InMemoryFS) GetMD(ctx context.Context, ref *provider.Reference, _ []string) (*provider.ResourceInfo, error) {
	user := ctxpkg.ContextMustGetUser(ctx)
	if infos, ok := fs.Resources[user.Id.OpaqueId]; ok {
		if info, ok := infos[ref.ResourceId.OpaqueId]; ok {
			return info, nil
		}
	}
	return nil, errtypes.NotFound(ref.ResourceId.OpaqueId)
}
