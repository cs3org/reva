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

package backend

import (
	"path"
	"strings"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// FindSpace returns the space of spaces that ref names, or a not-found
// *StatusError. ref is a path in the space (the space with the deepest root
// that holds it), a space id with or without its storage id, a name, or home
// for the personal space.
func FindSpace(spaces []*provider.StorageSpace, ref string) (*provider.StorageSpace, error) {
	var found *provider.StorageSpace
	if strings.HasPrefix(ref, "/") {
		p := path.Clean(ref)
		for _, s := range spaces {
			root := s.GetRootInfo().GetPath()
			if root == "" {
				continue
			}
			root = path.Clean(root)
			if (p == root || strings.HasPrefix(p, strings.TrimSuffix(root, "/")+"/")) &&
				(found == nil || len(root) > len(path.Clean(found.GetRootInfo().GetPath()))) {
				found = s
			}
		}
	} else {
		for _, s := range spaces {
			id := s.GetId().GetOpaqueId()
			_, spaceID, _ := strings.Cut(id, "$")
			if id == ref || spaceID == ref || s.GetName() == ref {
				found = s
				break
			}
		}
		if found == nil && ref == "home" {
			for _, s := range spaces {
				if s.GetSpaceType() == "personal" {
					found = s
					break
				}
			}
		}
	}
	if found == nil {
		return nil, &StatusError{Code: rpc.Code_CODE_NOT_FOUND, Msg: "space " + ref + " not found"}
	}
	return found, nil
}
