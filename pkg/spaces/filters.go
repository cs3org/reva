// Copyright 2018-2024 CERN
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
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
)

type ListStorageSpaceFilter struct {
	filters []*providerpb.ListStorageSpacesRequest_Filter
}

func (f ListStorageSpaceFilter) ByID(id *providerpb.StorageSpaceId) ListStorageSpaceFilter {
	f.filters = append(f.filters, &providerpb.ListStorageSpacesRequest_Filter{
		Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_ID,
		Term: &providerpb.ListStorageSpacesRequest_Filter_Id{
			Id: id,
		},
	})
	return f
}

func (f ListStorageSpaceFilter) ByOwner(owner *userpb.UserId) ListStorageSpaceFilter {
	f.filters = append(f.filters, &providerpb.ListStorageSpacesRequest_Filter{
		Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_OWNER,
		Term: &providerpb.ListStorageSpacesRequest_Filter_Owner{
			Owner: owner,
		},
	})
	return f
}

func (f ListStorageSpaceFilter) BySpaceType(spaceType SpaceType) ListStorageSpaceFilter {
	f.filters = append(f.filters, &providerpb.ListStorageSpacesRequest_Filter{
		Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE,
		Term: &providerpb.ListStorageSpacesRequest_Filter_SpaceType{
			SpaceType: spaceType.AsString(),
		},
	})
	return f
}

func (f ListStorageSpaceFilter) ByPath(path string) ListStorageSpaceFilter {
	f.filters = append(f.filters, &providerpb.ListStorageSpacesRequest_Filter{
		Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_PATH,
		Term: &providerpb.ListStorageSpacesRequest_Filter_Path{
			Path: path,
		},
	})
	return f
}

func (f ListStorageSpaceFilter) ByUser(user *userpb.UserId) ListStorageSpaceFilter {
	f.filters = append(f.filters, &providerpb.ListStorageSpacesRequest_Filter{
		Type: providerpb.ListStorageSpacesRequest_Filter_TYPE_USER,
		Term: &providerpb.ListStorageSpacesRequest_Filter_User{
			User: user,
		},
	})
	return f
}

func (f ListStorageSpaceFilter) List() []*providerpb.ListStorageSpacesRequest_Filter {
	return f.filters
}

// WithProjectStatus sets the project status in the Opaque map of a ListStorageSpacesRequest.
// Valid status values are: "creating", "active", "archiving", "archived".
// If no status is set, the default is "active".
func WithProjectStatus(req *providerpb.ListStorageSpacesRequest, status string) *providerpb.ListStorageSpacesRequest {
	if req.Opaque == nil {
		req.Opaque = &types.Opaque{
			Map: make(map[string]*types.OpaqueEntry),
		}
	} else if req.Opaque.Map == nil {
		req.Opaque.Map = make(map[string]*types.OpaqueEntry)
	}

	req.Opaque.Map[OpaqueKeyProjectStatus] = &types.OpaqueEntry{
		Decoder: "plain",
		Value:   []byte(status),
	}

	return req
}
