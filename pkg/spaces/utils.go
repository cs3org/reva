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
	"fmt"
	"path/filepath"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// EncodeStorageSpaceID encodes storage ID and space ID.
// In case of empty space ID, the path is used to create an identifier
// in the format <storage_id>$<space_id>, where base32(<path>) is the space ID.
func EncodeStorageSpaceID(storageID, spaceID string) string {
	return fmt.Sprintf("%s$%s", storageID, spaceID)
}

// DecodeStorageSpaceID returns the components of the storage + space ID.
// This ID is expected to be in the format <storage_id>$base32(<path>).
func DecodeStorageSpaceIDToPath(raw string) (storageID, path string, ok bool) {
	storageID, spaceId, ok := decodeStorageSpaceID(raw)
	if !ok {
		return "", "", false
	}
	path, err := DecodeSpaceID(spaceId)
	if err != nil {
		return "", "", false
	}
	return storageID, path, true
}

// DecodeStorageSpaceID returns the components of the storage + space ID.
// This ID is expected to be in the format <storage_id>$base32(<path>).
func decodeStorageSpaceID(raw string) (storageID, spaceId string, ok bool) {
	// The input is expected to be in the form of <storage_id>$<base32(<path>)
	s := strings.SplitN(raw, "$", 2)
	if len(s) != 2 {
		return "", "", false
	}

	return s[0], s[1], true
}

func EncodeSpaceID(spacePath string) string {
	return base32.StdEncoding.EncodeToString([]byte(spacePath))
}

func DecodeSpaceID(spaceId string) (string, error) {
	res, err := base32.StdEncoding.DecodeString(spaceId)
	if err != nil {
		return "", err
	}
	return string(res), nil
}

// TODO(lopresti) drop this in favor of having OCM received shares live in their space (however it is named)
func EncodeOCMShareID(ShareID string) string {
	return fmt.Sprintf("ocm-received$%s", base32.StdEncoding.EncodeToString([]byte("/ocm-received/"+ShareID)))
}

// EncodeToStringifiedResourceID encodes the provided resource ID as a string,
// in the format <storage_id>$<space_id>!<item_id>.
// If space_id or opaque_id is not set on the ResourceId,
// then this part will not be encoded
func EncodeToStringifiedResourceID(r *provider.ResourceId) string {
	if r.SpaceId == "" {
		return fmt.Sprintf("%s!%s", r.StorageId, r.OpaqueId)
	} else if r.OpaqueId == "" {
		return fmt.Sprintf("%s$%s", r.StorageId, r.SpaceId)
	}
	return fmt.Sprintf("%s$%s!%s", r.StorageId, r.SpaceId, r.OpaqueId)
}

// Decode resourceID returns the components of the space ID.
// The resource ID is expected to be in the form of <storage_id>$base32(<path>)!<item_id>.
func DecodeToResourceID(raw string) (storageID, spacePath, itemID string, ok bool) {
	// The input is expected to be in the form of <storage_id>$base32(<path>)!<item_id>
	rid, ok := ParseResourceID(raw)
	if !ok {
		return "", "", "", false
	}
	path, err := DecodeSpaceID(rid.SpaceId)
	if err != nil {
		return "", "", "", false
	}
	return rid.StorageId, path, rid.OpaqueId, true
}

// ParseResourceID converts the encoded resource id in a CS3API ResourceId.
func ParseResourceID(raw string) (*provider.ResourceId, bool) {
	// 	// The input is expected to be in the form of <storage_id>$base32(<path>)!<item_id>
	s := strings.SplitN(raw, "!", 2)
	if len(s) != 2 {
		return nil, false
	}
	storageID, spaceID, ok := decodeStorageSpaceID(s[0])
	if !ok {
		return nil, false
	}
	itemID := s[1]

	return &provider.ResourceId{
		StorageId: storageID,
		SpaceId:   spaceID,
		OpaqueId:  itemID,
	}, true
}

// Returns the path relative to the space root.
func PathRelativeToSpaceRoot(info *provider.ResourceInfo) (relativePath string, err error) {
	spacePath, err := DecodeSpaceID(info.Id.SpaceId)
	if err != nil {
		return "", err
	}

	return filepath.Rel(spacePath, info.Path)
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
