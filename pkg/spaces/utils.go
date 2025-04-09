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
	"encoding/base32"
	"errors"
	"fmt"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// DecodeSpaceID returns the components of the space ID.
// The space ID is expected to be in the format <storage_id>$base32(<path>).
func DecodeSpaceID(raw string) (storageID, path string, ok bool) {
	// The input is expected to be in the form of <storage_id>$<base32(<path>)
	s := strings.SplitN(raw, "$", 2)
	if len(s) != 2 {
		return "", "", false
	}

	storageID = s[0]
	encodedPath := s[1]
	p, err := base32.StdEncoding.DecodeString(encodedPath)
	if err != nil {
		return "", "", false
	}

	path = string(p)
	return storageID, path, true
}

// Decode resourceID returns the components of the space ID.
// The resource ID is expected to be in the form of <storage_id>$<base32(<path>)!<item_id>.
func DecodeResourceID(raw string) (storageID, path, itemID string, ok bool) {
	// The input is expected to be in the form of <storage_id>$base32(<path>)!<item_id>
	s := strings.SplitN(raw, "!", 2)
	if len(s) != 2 {
		return "", "", "", false
	}
	itemID = s[1]
	storageID, path, ok = DecodeSpaceID(s[0])
	return storageID, path, itemID, ok
}

// ParseResourceID converts the encoded resource id in a CS3API ResourceId.
func ParseResourceID(raw string) (*provider.ResourceId, bool) {
	storageID, path, itemID, ok := DecodeResourceID(raw)
	if !ok {
		return nil, false
	}
	return &provider.ResourceId{
		StorageId: storageID,
		SpaceId:   path,
		OpaqueId:  itemID,
	}, true
}

// EncodeResourceID encodes the provided resource ID as a string,
// in the format <storage_id>$<space_id>!<item_id>.
func EncodeResourceID(r *provider.ResourceId) string {
	spaceID := EncodeSpaceID(r.StorageId, r.SpaceId)
	return fmt.Sprintf("%s!%s", spaceID, r.OpaqueId)
}

func EncodeResourceInfo(md *provider.ResourceInfo) (spaceId string, err error) {
	if md.Id.SpaceId != "" {
		return fmt.Sprintf("%s$%s!%s", md.Id.StorageId, md.Id.SpaceId, md.Id.OpaqueId), nil
	} else if md.Path != "" {
		encodedPath := base32.StdEncoding.EncodeToString([]byte(md.Path))
		return fmt.Sprintf("%s$%s!%s", md.Id.StorageId, encodedPath, md.Id.OpaqueId), nil
	} else {
		return "", errors.New("resourceInfo must contain a spaceID or a path")
	}
}

// EncodeSpaceID encodes storage ID and path to create a space ID,
// in the format <storage_id>$base32(<path>).
func EncodeSpaceID(storageID, path string) string {
	if path == "" {
		return storageID
	}

	encodedPath := base32.StdEncoding.EncodeToString([]byte(path))
	return fmt.Sprintf("%s$%s", storageID, encodedPath)
}

func RelativePathToSpaceID(info *provider.ResourceInfo) string {
	return strings.TrimPrefix(info.Path, info.Id.SpaceId)
}

func ResourceIdToString(id *provider.ResourceId) string {
	return fmt.Sprintf("%s!%s", id.StorageId, id.OpaqueId)
}

func ResourceIdFromString(s string) (*provider.ResourceId, error) {
	parts := strings.Split(s, "!")
	if len(parts) != 2 {
		return nil, fmt.Errorf("string does not have right format: should be storageid!opaqueid, got %s", s)
	}
	return &provider.ResourceId{
		StorageId: parts[0],
		OpaqueId:  parts[1],
	}, nil
}
